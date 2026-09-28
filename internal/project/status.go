package project

import (
	"sort"

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
				c.SetDiff = r.setDiff(old.Hash, f.Path)
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

func (r *Repo) setDiff(oldHash, rel string) *diff.SetDiff {
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
		return nil
	}
	return diff.Diff(old, cur)
}

func sortChanges(cs []Change) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Path < cs[j].Path })
}
