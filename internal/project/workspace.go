package project

import (
	"errors"
	"sort"

	"dawgit/internal/remote"
)

// TrackEdit is one piece of unsaved work: a track (or a set-wide setting)
// changed since the version the workspace is on.
type TrackEdit struct {
	Set     string `json:"set"`
	TrackID string `json:"track_id,omitempty"` // empty for set-wide changes
	Name    string `json:"name"`
	Change  string `json:"change"` // "added" | "removed" | "modified"
}

// LocalEdits lists unsaved track-level changes in the working sets.
func (r *Repo) LocalEdits() ([]TrackEdit, error) {
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	return EditsIn(changes), nil
}

// EditsIn lists the track-level changes in sets among changes (from Status).
func EditsIn(changes []Change) []TrackEdit {
	var edits []TrackEdit
	for _, c := range changes {
		if !isSet(c.Path) {
			continue
		}
		switch c.Status {
		case "added":
			edits = append(edits, TrackEdit{Set: c.Path, Name: "(new set)", Change: "added"})
		case "modified":
			if c.SetDiff == nil {
				continue
			}
			for _, g := range c.SetDiff.GlobalChanges {
				edits = append(edits, TrackEdit{Set: c.Path, Name: g, Change: "modified"})
			}
			for _, tc := range c.SetDiff.TrackChanges {
				edits = append(edits, TrackEdit{Set: c.Path, TrackID: tc.TrackID, Name: tc.Name, Change: tc.Status})
			}
		}
	}
	return edits
}

// IncomingVersions lists versions on the server branch that this workspace
// does not have, newest first.
// It asks only for this branch's head: one small read, cheap enough to poll.
func (r *Repo) IncomingVersions() ([]*Manifest, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	target, err := branchHead(c, r.Config.ProjectID, r.BranchName())
	if err != nil {
		return nil, err
	}
	if target != "" && target != r.Latest() {
		if err := r.fetchSnapshots(c, target); err != nil {
			return nil, err
		}
	}
	return r.IncomingFrom(map[string]string{r.BranchName(): target})
}

// branchHead is one branch's head ("" if none): a single read where the
// backend can (storage bills listing more than reading).
func branchHead(c remote.Backend, pid, name string) (string, error) {
	if b, ok := c.(interface {
		BranchHead(pid, name string) (string, error)
	}); ok {
		return b.BranchHead(pid, name)
	}
	heads, err := c.Branches(pid)
	if errors.Is(err, remote.ErrNotFound) {
		return "", nil
	}
	return heads[name], err
}

// IncomingFrom is IncomingVersions for branch heads already fetched (see
// FetchTeam): no network.
func (r *Repo) IncomingFrom(heads map[string]string) ([]*Manifest, error) {
	target := heads[r.BranchName()]
	if target == "" || target == r.Latest() || !r.HasSnapshot(target) {
		return nil, nil
	}
	have, err := r.ancestors(r.Latest())
	if err != nil {
		return nil, err
	}
	incoming, err := r.ancestors(target)
	if err != nil {
		return nil, err
	}
	var out []*Manifest
	for id := range incoming {
		if !have[id] {
			m, err := r.Load(id)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}
