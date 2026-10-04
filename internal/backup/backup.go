// Package backup copies a team's storage into a folder (an external drive, a
// NAS) or another bucket: every key as it is, at the same path, so the
// backup is the team's storage as of the last run. Runs are incremental and
// never delete: what the team's storage cleans up (a deleted project, say)
// stays in the backup.
//
// Contents (objects/, chunked/, version records) never change once written:
// copied once. The few small keys that do change (branches, members, the
// team's name, workspaces) are copied again when they changed, first: the
// team writes contents before it moves a branch, so every version a copied
// branch names is in the backup by the end of the run. Each run also
// records where every branch was (runs/<time>.json), to go back to any run.
package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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

// Run backs up src into d (claimed for the team: see Claim).
func Run(src Source, d Dest, progress Progress) (*Report, error) {
	if err := d.Begin(); err != nil {
		return nil, err
	}
	defer d.End()
	if _, err := d.Read(readmeFile); errors.Is(err, fs.ErrNotExist) {
		d.Write(readmeFile, []byte(readme))
	}
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
	if err := copyAll(src, d, changing, rep, progress, changed); err != nil {
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
	if err := copyAll(src, d, contents, rep, progress, missing); err != nil {
		return nil, err
	}
	if err := writeRun(d, rep.Run, changing); err != nil {
		return nil, err
	}
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

// plain: key is a plain relative path (it can't climb out of a folder).
func plain(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, ":") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// missing: copy contents not in the backup yet (or cut short).
func missing(d Dest, it remote.Item) bool {
	size, _, ok := d.Stat(it.Key)
	return !ok || size != it.Size
}

// changed: copy a changing key when the team's copy is newer than the
// backup's (or a different size).
func changed(d Dest, it remote.Item) bool {
	size, modified, ok := d.Stat(it.Key)
	return !ok || size != it.Size || it.Modified.Truncate(time.Second).After(modified)
}

func copyAll(src Source, d Dest, items []remote.Item, rep *Report, progress Progress,
	need func(Dest, remote.Item) bool) error {
	var todo []remote.Item
	var total int64
	for _, it := range items {
		if plain(it.Key) && need(d, it) {
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
				err := copyKey(src, d, it)
				mu.Lock()
				if err != nil && first == nil {
					first = fmt.Errorf("%s: %w", it.Key, err)
				}
				if err == nil {
					rep.Copied++
					rep.CopiedBytes += it.Size
				}
				mu.Unlock()
				if progress != nil {
					progress(done.Add(it.Size), total)
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

func copyKey(src Source, d Dest, it remote.Item) error {
	body, err := src.Open(it.Key)
	if err != nil {
		return err
	}
	defer body.Close()
	return d.Put(it.Key, &exact{r: body, left: it.Size}, it)
}

// exact reads exactly left bytes: fewer or more is an error, so a copy cut
// short (or a key changed under it) never stands as a good one.
type exact struct {
	r    io.Reader
	left int64
}

func (e *exact) Read(p []byte) (int, error) {
	if e.left <= 0 {
		var one [1]byte
		if n, _ := e.r.Read(one[:]); n > 0 {
			return 0, errors.New("longer than listed")
		}
		return 0, io.EOF
	}
	if int64(len(p)) > e.left {
		p = p[:e.left]
	}
	n, err := e.r.Read(p)
	e.left -= int64(n)
	if err == io.EOF && e.left > 0 {
		return n, io.ErrUnexpectedEOF
	}
	if err == io.EOF {
		err = nil
	}
	return n, err
}

// RunRecord is where every branch was at a run (runs/<time>.json).
type RunRecord struct {
	Time     string                       `json:"time"`
	Branches map[string]map[string]string `json:"branches"` // project id -> branch -> version
}

// writeRun records the branches as the backup has them now.
func writeRun(d Dest, run string, changing []remote.Item) error {
	rec := RunRecord{Time: run, Branches: map[string]map[string]string{}}
	for _, it := range changing {
		parts := strings.Split(it.Key, "/") // projects/<id>/branches/<name>
		if len(parts) != 4 || parts[0] != "projects" || parts[2] != "branches" {
			continue
		}
		data, err := d.Read(it.Key)
		if err != nil {
			continue
		}
		if rec.Branches[parts[1]] == nil {
			rec.Branches[parts[1]] = map[string]string{}
		}
		rec.Branches[parts[1]][parts[3]] = strings.TrimSpace(string(data))
	}
	data, _ := json.MarshalIndent(rec, "", "  ")
	return d.Write("runs/"+run+".json", data)
}

const readmeFile = "README.txt"

const readme = `This is a backup of a DAWGit team's storage, made by the DAWGit app.

It holds every project of the team, with all its versions: the same files, at
the same paths, as the team's storage (an S3 bucket such as Cloudflare R2).
Backups only add: what the team deleted stays here.

runs/      where every branch was at each backup, to go back to any of them
objects/   file contents (some compressed, some in pieces: read by DAWGit)
projects/  each project's versions and branches

To restore, copy everything here (except runs/, README.txt and
dawgit-backup.json) into an empty bucket and connect DAWGit to it.
Don't change files here by hand.
`

// markFile says whose backup it is.
const markFile = "dawgit-backup.json"

type mark struct {
	Team string `json:"team"` // the team's id
	Name string `json:"name"`
}

// ErrOtherTeam: the place holds another team's backup.
var ErrOtherTeam = errors.New("this holds another team's backup")

// ErrNotEmpty: the place has other things in it.
var ErrNotEmpty = errors.New("choose an empty folder or bucket, or one with this team's backup")

// ErrMissing: the backup isn't there (a drive unplugged, a bucket emptied).
var ErrMissing = errors.New("the backup isn't there")

// Claim makes d team teamID's backup: an empty place (a folder is made if
// need be), or one already holding that team's backup.
func Claim(d Dest, teamID, name string) error {
	if data, err := d.Read(markFile); err == nil {
		var m mark
		if json.Unmarshal(data, &m) == nil && m.Team == teamID {
			return nil
		}
		return ErrOtherTeam
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	empty, err := d.Empty()
	if err != nil {
		return err
	}
	if !empty {
		return ErrNotEmpty
	}
	data, _ := json.MarshalIndent(mark{Team: teamID, Name: name}, "", "  ")
	return d.Write(markFile, data)
}

// Claimed: d is team teamID's backup (ErrMissing if it isn't there: a
// drive unplugged).
func Claimed(d Dest, teamID string) error {
	data, err := d.Read(markFile)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrMissing
	}
	if err != nil {
		return err
	}
	var m mark
	if json.Unmarshal(data, &m) != nil || m.Team != teamID {
		return ErrOtherTeam
	}
	return nil
}
