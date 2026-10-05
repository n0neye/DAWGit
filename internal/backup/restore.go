package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"dawgit/internal/remote"
)

// Restoring copies a backup back into a team's storage: only what the
// storage lacks, never overwriting anything, so it can't undo a teammate's
// work. It brings back deleted projects, lost files, or a whole team (into
// a new, empty bucket the team is set up on).
//
// From the latest state, or from a run (runs/<time>.json): then projects
// made after that run stay out, and branches come back where they were at
// that run. Branches the team still has are left as they are either way.
//
// Branches go last, so none ever names a version whose files aren't back.

// Target is the team's storage, written to (remote.BucketBackend).
type Target interface {
	List(prefix string) ([]remote.Item, error)
	Put(key string, r io.Reader, size int64) error
	PutNew(key string, data []byte) (bool, error)
}

// Plan is what a restore would bring back.
type Plan struct {
	Team     string            `json:"team"` // the team the backup is of (its name then)
	Run      string            `json:"run"`  // "" for the latest
	Runs     []string          `json:"runs"` // the backup's runs, newest first
	Projects []PlanProject     `json:"projects"`
	Branches int               `json:"branches"` // branches back in projects the team still has
	Files    int               `json:"files"`
	Bytes    int64             `json:"bytes"`
	keys     []remote.Item     // to copy, in order
	heads    map[string]string // branch key -> version, for a run
}

// PlanProject is a project the restore brings back whole.
type PlanProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Versions int    `json:"versions"`
}

// Empty: nothing to bring back.
func (p *Plan) Empty() bool { return len(p.keys) == 0 }

// ErrNotBackup: the place holds no DAWGit backup.
var ErrNotBackup = errors.New("this isn't a DAWGit backup")

// ErrNoRun: no such run in the backup.
var ErrNoRun = errors.New("the backup has no such run")

// notRestored: the backup's own files, and keys that only matter where
// they were made.
func notRestored(key string) bool {
	if key == markFile || key == readmeFile || !plain(key) {
		return true
	}
	for _, p := range []string{"runs/", ".tmp/", "check/", "gc/", "backups/"} {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// Runs lists the runs in backup src, newest first.
func Runs(src Dest) ([]string, error) {
	items, err := src.List()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		if name, ok := strings.CutPrefix(it.Key, "runs/"); ok && strings.HasSuffix(name, ".json") {
			out = append(out, strings.TrimSuffix(name, ".json"))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out, nil
}

// MakePlan works out what restoring src (as of run, "" for the latest)
// into the team's storage would bring back.
func MakePlan(src Dest, team Target, run string) (*Plan, error) {
	data, err := src.Read(markFile)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotBackup
	}
	if err != nil {
		return nil, err
	}
	var m mark
	if json.Unmarshal(data, &m) != nil {
		return nil, ErrNotBackup
	}
	p := &Plan{Team: m.Name, Run: run, Projects: []PlanProject{}}
	if p.Runs, err = Runs(src); err != nil {
		return nil, err
	}
	var rec *RunRecord
	if run != "" {
		data, err := src.Read("runs/" + run + ".json")
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoRun
		}
		if err != nil {
			return nil, err
		}
		rec = &RunRecord{}
		if err := json.Unmarshal(data, rec); err != nil {
			return nil, fmt.Errorf("runs/%s.json: %w", run, err)
		}
		p.heads = map[string]string{}
	}

	items, err := src.List()
	if err != nil {
		return nil, err
	}
	have, err := team.List("")
	if err != nil {
		return nil, err
	}
	has := make(map[string]bool, len(have))
	for _, it := range have {
		has[it.Key] = true
	}

	// By kind, in the order they go back.
	var contents, records, branches []remote.Item
	versions := map[string]int{} // project id -> versions coming back
	for _, it := range items {
		if notRestored(it.Key) || has[it.Key] {
			continue
		}
		parts := strings.Split(it.Key, "/")
		if parts[0] == "projects" && len(parts) >= 3 {
			pid := parts[1]
			if rec != nil && rec.Branches[pid] == nil && !has["projects/"+pid+"/project.json"] {
				continue // made after the run
			}
			if len(parts) == 4 && parts[2] == "branches" {
				if rec != nil {
					head, ok := rec.Branches[pid][parts[3]]
					if !ok {
						continue // made after the run
					}
					p.heads[it.Key] = head
				}
				branches = append(branches, it)
				continue
			}
			if parts[2] == "snapshots" {
				versions[pid]++
			}
		}
		if immutable(it.Key) {
			contents = append(contents, it)
		} else {
			records = append(records, it)
		}
	}
	// project.json last among the records: a project shows up once whole.
	sort.SliceStable(records, func(i, j int) bool {
		return !strings.HasSuffix(records[i].Key, "/project.json") && strings.HasSuffix(records[j].Key, "/project.json")
	})
	p.keys = append(append(append(p.keys, contents...), records...), branches...)
	for _, it := range p.keys {
		p.Files++
		p.Bytes += it.Size
	}

	back := map[string]bool{}
	for _, it := range records {
		if pid, ok := projectOf(it.Key, "project.json"); ok {
			back[pid] = true
			name := pid
			if data, err := src.Read(it.Key); err == nil {
				var pr remote.Project
				if json.Unmarshal(data, &pr) == nil && pr.Name != "" {
					name = pr.Name
				}
			}
			p.Projects = append(p.Projects, PlanProject{ID: pid, Name: name, Versions: versions[pid]})
		}
	}
	for _, it := range branches {
		if pid, _, _ := strings.Cut(strings.TrimPrefix(it.Key, "projects/"), "/"); !back[pid] {
			p.Branches++
		}
	}
	sort.Slice(p.Projects, func(i, j int) bool { return p.Projects[i].Name < p.Projects[j].Name })
	return p, nil
}

// projectOf: the project id of projects/<id>/<name>.
func projectOf(key, name string) (string, bool) {
	parts := strings.Split(key, "/")
	if len(parts) == 3 && parts[0] == "projects" && parts[2] == name {
		return parts[1], true
	}
	return "", false
}

// Restore carries out plan p: contents first, then records, branches last.
func Restore(src Dest, team Target, p *Plan, progress Progress) (*Report, error) {
	rep := &Report{Run: p.Run, Keys: p.Files, TotalBytes: p.Bytes}
	var done atomic.Int64
	tell := func(n int64) {
		if progress != nil {
			progress(done.Add(n), p.Bytes)
		}
	}
	// Contents in parallel; the rest in order.
	split := 0
	for split < len(p.keys) && immutable(p.keys[split].Key) {
		split++
	}
	var mu sync.Mutex
	var first error
	ch := make(chan remote.Item)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				err := restoreKey(src, team, it)
				mu.Lock()
				if err != nil && first == nil {
					first = fmt.Errorf("%s: %w", it.Key, err)
				} else if err == nil {
					rep.Copied++
					rep.CopiedBytes += it.Size
				}
				mu.Unlock()
				tell(it.Size)
			}
		}()
	}
	for _, it := range p.keys[:split] {
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
	if first != nil {
		return rep, first
	}
	for _, it := range p.keys[split:] {
		var data []byte
		if head, ok := p.heads[it.Key]; ok {
			data = []byte(head + "\n")
		} else {
			r, err := src.Open(it.Key)
			if err != nil {
				return rep, fmt.Errorf("%s: %w", it.Key, err)
			}
			data, err = io.ReadAll(&exact{r: r, left: it.Size})
			r.Close()
			if err != nil {
				return rep, fmt.Errorf("%s: %w", it.Key, err)
			}
		}
		created, err := team.PutNew(it.Key, data)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", it.Key, err)
		}
		if created {
			rep.Copied++
			rep.CopiedBytes += int64(len(data))
		}
		tell(it.Size)
	}
	return rep, nil
}

func restoreKey(src Dest, team Target, it remote.Item) error {
	r, err := src.Open(it.Key)
	if err != nil {
		return err
	}
	defer r.Close()
	return team.Put(it.Key, &exact{r: r, left: it.Size}, it.Size)
}
