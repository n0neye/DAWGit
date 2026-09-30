package project

import (
	"sort"
	"sync"

	"dawgit/internal/als"
	"dawgit/internal/diff"
)

type Change struct {
	Path   string
	Status string // "added" | "modified" | "deleted"
	// SetDiff is the semantic diff for a modified Live Set.
	SetDiff *diff.SetDiff
}

// Status compares the working files with HEAD.
func (r *Repo) Status() ([]Change, error) {
	ix := r.loadIndex()
	files, err := r.workingFiles(ix)
	if err != nil {
		return nil, err
	}
	defer ix.save()

	var head map[string]FileEntry
	if id := r.Head(); id != "" {
		m, err := r.Load(id)
		if err != nil {
			return nil, err
		}
		head = m.FileMap()
	}
	var out []Change
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
		old, ok := head[f.Path]
		switch {
		case !ok:
			out = append(out, Change{Path: f.Path, Status: "added"})
		case old.Hash != f.Hash:
			c := Change{Path: f.Path, Status: "modified"}
			if isSet(f.Path) {
				c.SetDiff = r.workingSetDiff(old.Hash, f.Hash, f.Path)
			}
			out = append(out, c)
		}
	}
	for path := range head {
		if !seen[path] {
			out = append(out, Change{Path: path, Status: "deleted"})
		}
	}
	sortChanges(out)
	return out, nil
}

// workingSetDiff compares a set in the project folder (whose content hash is
// newHash) with a stored version of it. Parsing sets is the slow part of
// reading a project, and the same pair comes up again and again (every
// status and every backup until the next commit), so diffs are kept by the
// pair of contents. Diffs are shared: callers must not change them.
func (r *Repo) workingSetDiff(oldHash, newHash, rel string) *diff.SetDiff {
	key := oldHash + ":" + newHash
	if d := diffCache.get(key); d != nil {
		return d
	}
	data, err := r.Store.Read(oldHash)
	if err != nil {
		return nil
	}
	old, err := als.FromGzip(data)
	if err != nil {
		return nil
	}
	cur, err := als.Load(r.Abs(rel))
	if err != nil {
		return nil // e.g. Live is writing the file right now
	}
	d := diff.Diff(old, cur)
	diffCache.put(key, d)
	return d
}

// diffCache holds the last few set diffs (a handful of changed sets at a time).
var diffCache = &setDiffCache{max: 32}

type setDiffCache struct {
	mu    sync.Mutex
	max   int
	keys  []string // oldest first
	diffs map[string]*diff.SetDiff
}

func (c *setDiffCache) get(key string) *diff.SetDiff {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.diffs[key]
}

func (c *setDiffCache) put(key string, d *diff.SetDiff) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.diffs == nil {
		c.diffs = map[string]*diff.SetDiff{}
	}
	if _, ok := c.diffs[key]; ok {
		return
	}
	if len(c.keys) >= c.max {
		delete(c.diffs, c.keys[0])
		c.keys = c.keys[1:]
	}
	c.keys = append(c.keys, key)
	c.diffs[key] = d
}

func sortChanges(cs []Change) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Path < cs[j].Path })
}
