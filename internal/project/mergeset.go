package project

import (
	"bytes"
	"fmt"

	"dawgit/internal/als"
	"dawgit/internal/merge"
)

// mergeSet merges one Live Set changed on both sides, track by track. It
// returns the merged file entry, a log, and conflicts left unresolved (only
// with strategy "fail").
func (r *Repo) mergeSet(path, baseHash, oursHash, theirsHash, strategy string) (FileEntry, []string, []string, error) {
	load := func(h string) (*als.LiveSet, error) {
		data, err := r.Store.Read(h)
		if err != nil {
			return nil, err
		}
		return als.FromGzip(data)
	}
	var sets [3]*als.LiveSet
	for i, h := range []string{baseHash, oursHash, theirsHash} {
		s, err := load(h)
		if err != nil {
			return FileEntry{}, nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		sets[i] = s
	}
	res, err := merge.Merge(sets[0], sets[1], sets[2], strategy)
	if err != nil {
		return FileEntry{}, nil, nil, err
	}
	if len(res.Issues) > 0 {
		return FileEntry{}, nil, nil, fmt.Errorf("%s: merge produced an invalid set:\n%s", path, res.Report())
	}
	var log, conflicts []string
	for _, l := range res.Log {
		log = append(log, path+": "+l)
	}
	for _, c := range res.Conflicts {
		if strategy == "fail" {
			conflicts = append(conflicts, fmt.Sprintf("%s: %s %s", path, c.Unit, c.Description))
		} else {
			log = append(log, fmt.Sprintf("%s: %s %s -> %s", path, c.Unit, c.Description, c.Resolution))
		}
	}
	h, n, err := r.Store.Put(bytes.NewReader(res.Merged.Gzip()))
	if err != nil {
		return FileEntry{}, nil, nil, err
	}
	return FileEntry{Path: path, Hash: h, Size: n}, log, conflicts, nil
}
