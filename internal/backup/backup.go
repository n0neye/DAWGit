// Package backup copies a team's storage into a folder (an external drive, a
// NAS): every key as it is, at the same path, so the folder is the team's
// storage as of the last run. Runs are incremental and never delete: what
// the team's storage cleans up (a deleted project, say) stays in the backup.
//
// Contents (objects/, chunked/, version records) never change once written:
// copied once. The few small keys that do change (branches, members, the
// team's name, workspaces) are copied again when they differ, first: the
// team writes contents before it moves a branch, so every version a copied
// branch names is in the backup by the end of the run. Each run also
// records where every branch was (runs/<time>.json), to go back to any run.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"dawgit/internal/remote"
)

// Source is a team's storage, read key by key (remote.S3Backend).
type Source interface {
	List(prefix string) ([]remote.Item, error)
	Open(key string) (io.ReadCloser, error)
}

// Report is what a run did.
type Report struct {
	Keys        int   // in the team's storage
	Copied      int   // copied this run
	CopiedBytes int64 //
	TotalBytes  int64 // everything in the backup that the team's storage has
	Run         string
}

// Progress hears about a run: bytes copied of the bytes to copy.
type Progress func(done, total int64)

// workers copy at once.
const workers = 8

// skipped are keys a backup leaves out: cleanup's bookkeeping, and other
// members' backup records.
var skipped = []string{"gc/", "backups/"}

// immutable: written once, never changed.
func immutable(key string) bool {
	return strings.HasPrefix(key, "objects/") || strings.HasPrefix(key, "chunked/") ||
		strings.Contains(key, "/snapshots/")
}

// Run backs up src into the folder dst.
func Run(src Source, dst string, progress Progress) (*Report, error) {
	if err := os.MkdirAll(filepath.Join(dst, ".tmp"), 0o755); err != nil {
		return nil, err
	}
	writeReadme(dst)
	rep := &Report{Run: time.Now().UTC().Format("20060102-150405")}

	// What changes, first (see the package doc).
	items, err := src.List("")
	if err != nil {
		return nil, err
	}
	var changing []remote.Item
	for _, it := range items {
		if !skip(it.Key) && !immutable(it.Key) {
			changing = append(changing, it)
		}
	}
	if err := copyAll(src, dst, changing, rep, progress, changed); err != nil {
		return nil, err
	}
	// Then the contents: listed again, so what the copied branches name is in.
	items, err = src.List("")
	if err != nil {
		return nil, err
	}
	var contents []remote.Item
	for _, it := range items {
		if skip(it.Key) {
			continue
		}
		rep.Keys++
		rep.TotalBytes += it.Size
		if immutable(it.Key) {
			contents = append(contents, it)
		}
	}
	if err := copyAll(src, dst, contents, rep, progress, missing); err != nil {
		return nil, err
	}
	if err := writeRun(dst, rep.Run); err != nil {
		return nil, err
	}
	os.RemoveAll(filepath.Join(dst, ".tmp"))
	return rep, nil
}

func skip(key string) bool {
	for _, p := range skipped {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// local is where key goes in dst; "" for a key that isn't a plain path.
func local(dst, key string) string {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, ":") {
		return ""
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return ""
		}
	}
	return filepath.Join(dst, filepath.FromSlash(key))
}

// missing: copy contents not in the backup yet (or cut short).
func missing(path string, it remote.Item) bool {
	fi, err := os.Stat(path)
	return err != nil || fi.Size() != it.Size
}

// changed: copy a changing key when it differs from the backup's copy.
func changed(path string, it remote.Item) bool {
	fi, err := os.Stat(path)
	return err != nil || fi.Size() != it.Size || !fi.ModTime().Equal(it.Modified.Truncate(time.Second))
}

func copyAll(src Source, dst string, items []remote.Item, rep *Report, progress Progress,
	need func(string, remote.Item) bool) error {
	var todo []remote.Item
	var total int64
	for _, it := range items {
		p := local(dst, it.Key)
		if p != "" && need(p, it) {
			todo = append(todo, it)
			total += it.Size
		}
	}
	if len(todo) == 0 {
		return nil
	}
	var done atomic.Int64
	var mu sync.Mutex
	var first error
	ch := make(chan remote.Item)
	var wg sync.WaitGroup
	for range min(workers, len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				n, err := copyKey(src, dst, it)
				mu.Lock()
				if err != nil && first == nil {
					first = fmt.Errorf("%s: %w", it.Key, err)
				}
				if err == nil {
					rep.Copied++
					rep.CopiedBytes += n
				}
				mu.Unlock()
				if progress != nil {
					progress(done.Add(n), total)
				}
			}
		}()
	}
	for _, it := range todo {
		mu.Lock()
		stop := first != nil
		mu.Unlock()
		if stop {
			break
		}
		ch <- it
	}
	close(ch)
	wg.Wait()
	return first
}

// copyKey copies one key into dst through a temporary file (a run that
// stops leaves no half file in place), keeping its time.
func copyKey(src Source, dst string, it remote.Item) (int64, error) {
	path := local(dst, it.Key)
	body, err := src.Open(it.Key)
	if err != nil {
		return 0, err
	}
	defer body.Close()
	tmp, err := os.CreateTemp(filepath.Join(dst, ".tmp"), "copy-*")
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(tmp, body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil && n != it.Size {
		err = fmt.Errorf("got %d bytes of %d", n, it.Size)
	}
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return 0, err
	}
	t := it.Modified.Truncate(time.Second)
	os.Chtimes(path, t, t)
	return n, nil
}

// RunRecord is where every branch was at a run (runs/<time>.json).
type RunRecord struct {
	Time     string                       `json:"time"`
	Branches map[string]map[string]string `json:"branches"` // project id -> branch -> version
}

func writeRun(dst, run string) error {
	rec := RunRecord{Time: run, Branches: map[string]map[string]string{}}
	projects, _ := os.ReadDir(filepath.Join(dst, "projects"))
	for _, p := range projects {
		dir := filepath.Join(dst, "projects", p.Name(), "branches")
		bs, _ := os.ReadDir(dir)
		for _, b := range bs {
			data, err := os.ReadFile(filepath.Join(dir, b.Name()))
			if err != nil {
				continue
			}
			if rec.Branches[p.Name()] == nil {
				rec.Branches[p.Name()] = map[string]string{}
			}
			rec.Branches[p.Name()][b.Name()] = strings.TrimSpace(string(data))
		}
	}
	data, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.MkdirAll(filepath.Join(dst, "runs"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "runs", run+".json"), data, 0o644)
}

// Runs lists the runs in a backup, newest first.
func Runs(dst string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(dst, "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

// Size is how many bytes the backup holds.
func Size(dst string) int64 {
	var n int64
	filepath.WalkDir(dst, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

func writeReadme(dst string) {
	p := filepath.Join(dst, "README.txt")
	if _, err := os.Stat(p); err == nil {
		return
	}
	os.WriteFile(p, []byte(`This folder is a backup of a DAWGit team's storage, made by the DAWGit app.

It holds every project of the team, with all its versions: the same files, at
the same paths, as the team's storage (an S3 bucket such as Cloudflare R2).
Backups only add: what the team deleted stays here.

runs/      where every branch was at each backup, to go back to any of them
objects/   file contents (some compressed, some in pieces: read by DAWGit)
projects/  each project's versions and branches

To restore, copy this folder's contents (except runs/ and README.txt) into an
empty bucket and connect DAWGit to it, or ask DAWGit to restore it.
Don't change files here by hand.
`), 0o644)
}

// markFile says whose backup a folder is.
const markFile = "dawgit-backup.json"

type mark struct {
	Team string `json:"team"` // the team's id
	Name string `json:"name"`
}

// ErrOtherTeam: the folder holds another team's backup.
var ErrOtherTeam = errors.New("this folder holds another team's backup")

// ErrNotEmpty: the folder has other things in it.
var ErrNotEmpty = errors.New("choose an empty folder, or one with this team's backup")

// Claim makes dst the backup folder of team teamID: an empty folder (made
// if need be), or one already holding that team's backup.
func Claim(dst, teamID, name string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if data, err := os.ReadFile(filepath.Join(dst, markFile)); err == nil {
		var m mark
		if json.Unmarshal(data, &m) == nil && m.Team == teamID {
			return nil
		}
		return ErrOtherTeam
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	for _, e := range entries {
		// What Windows and macOS put in any folder.
		if n := strings.ToLower(e.Name()); n != "desktop.ini" && n != ".ds_store" && n != "thumbs.db" {
			return ErrNotEmpty
		}
	}
	data, _ := json.MarshalIndent(mark{Team: teamID, Name: name}, "", "  ")
	return os.WriteFile(filepath.Join(dst, markFile), data, 0o644)
}

// Claimed: dst is team teamID's backup folder (it may be unplugged).
func Claimed(dst, teamID string) error {
	data, err := os.ReadFile(filepath.Join(dst, markFile))
	if err != nil {
		return err
	}
	var m mark
	if json.Unmarshal(data, &m) != nil || m.Team != teamID {
		return ErrOtherTeam
	}
	return nil
}
