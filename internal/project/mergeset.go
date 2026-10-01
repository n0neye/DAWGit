package project

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/handlers"
	"dawgit/internal/merge"
)

// mergeSet merges one Live Set changed on both sides, track by track. It
// returns the merged file entry, a log, and conflicts left unresolved.
func (r *Repo) mergeSet(path, baseHash, oursHash, theirsHash string, opts MergeOptions) (FileEntry, []string, []ConflictItem, error) {
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
	res, err := merge.MergeWith(sets[0], sets[1], sets[2], opts.forSet(path))
	if err != nil {
		return FileEntry{}, nil, nil, err
	}
	if len(res.Issues) > 0 {
		return FileEntry{}, nil, nil, fmt.Errorf("%s: merge produced an invalid set:\n%s", path, res.Report())
	}
	var log []string
	var conflicts []ConflictItem
	for _, l := range res.Log {
		log = append(log, path+": "+l)
	}
	for _, c := range res.Conflicts {
		if c.Unresolved {
			conflicts = append(conflicts, ConflictItem{Key: path + setKeySep + c.Key, File: path, Unit: c.Unit,
				Description: c.Description, CanKeepBoth: strings.HasPrefix(c.Key, "track:")})
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

// mergeWithHandler merges a file changed on both sides with the merge handler
// its preset names; ok is false when there is none or the changes collide.
func (r *Repo) mergeWithHandler(path, baseHash, oursHash, theirsHash string) (FileEntry, bool, error) {
	name := r.rules().Handler(path).Merge
	fn := handlers.Merge(name)
	if fn == nil {
		return FileEntry{}, false, nil
	}
	if err := r.ensureHashes([]string{baseHash, oursHash, theirsHash}); err != nil {
		return FileEntry{}, false, err
	}
	var data [3][]byte
	for i, h := range []string{baseHash, oursHash, theirsHash} {
		f, err := r.openObject(h)
		if err != nil {
			return FileEntry{}, false, err
		}
		data[i], err = io.ReadAll(f)
		f.Close()
		if err != nil {
			return FileEntry{}, false, err
		}
	}
	merged, clean, err := fn(data[0], data[1], data[2])
	if err != nil {
		return FileEntry{}, false, fmt.Errorf("%s: %s merge: %w", path, name, err)
	}
	if !clean {
		return FileEntry{}, false, nil
	}
	h, n, err := r.Store.Put(bytes.NewReader(merged))
	if err != nil {
		return FileEntry{}, false, err
	}
	return FileEntry{Path: path, Hash: h, Size: n}, true, nil
}

// setKeySep joins a set path and a merge key in conflict keys: "Song.als#track:14".
const setKeySep = "#"

// MergeOptions decide conflicts when merging versions.
type MergeOptions struct {
	// Strategy for conflicts without a resolution: fail | ours | theirs | both.
	Strategy string
	// Resolutions by ConflictItem.Key.
	Resolutions map[string]string
}

// Strategy is MergeOptions with one strategy for all conflicts.
func Strategy(s string) MergeOptions { return MergeOptions{Strategy: s} }

func (o MergeOptions) choice(key string) string {
	if r, ok := o.Resolutions[key]; ok {
		return r
	}
	if o.Strategy == "" {
		return "fail"
	}
	return o.Strategy
}

// forSet extracts the options for one set's track merge.
func (o MergeOptions) forSet(path string) merge.Options {
	opts := merge.Options{Strategy: o.Strategy, Resolutions: map[string]string{}}
	if opts.Strategy == "" {
		opts.Strategy = "fail"
	}
	prefix := path + setKeySep
	for k, v := range o.Resolutions {
		if strings.HasPrefix(k, prefix) {
			opts.Resolutions[strings.TrimPrefix(k, prefix)] = v
		}
	}
	return opts
}

// ConflictItem is one thing changed on both sides that needs a decision.
type ConflictItem struct {
	Key         string // pass back in MergeOptions.Resolutions
	File        string
	Unit        string // e.g. `AudioTrack "Bass"`, or the file itself
	Description string
	// CanKeepBoth: "both" keeps both versions (tracks and files).
	CanKeepBoth bool
}

func (c ConflictItem) String() string {
	if c.Unit == c.File {
		return c.File + ": " + c.Description
	}
	return c.File + ": " + c.Unit + " " + c.Description
}
