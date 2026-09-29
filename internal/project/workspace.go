package project

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"dawgit/internal/als"
	"dawgit/internal/diff"
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

// WorkspaceState is what a member's agent reports to the server.
type WorkspaceState struct {
	ID       string      `json:"id"`
	Author   string      `json:"author"`
	AuthorID string      `json:"author_id,omitempty"`
	Branch   string      `json:"branch"`
	Base     string      `json:"base"` // version the edits are relative to
	Updated  string      `json:"updated"`
	Edits    []TrackEdit `json:"edits"`
	// Files are backups of the modified sets (objects on the server).
	Files []FileEntry `json:"files,omitempty"`
}

func (w WorkspaceState) UpdatedTime() time.Time {
	t, _ := time.Parse(time.RFC3339, w.Updated)
	return t
}

// WorkspaceID identifies this copy of the project; created on first use.
func (r *Repo) WorkspaceID() (string, error) {
	if r.Config.WorkspaceID == "" {
		b := make([]byte, 16)
		rand.Read(b)
		r.Config.WorkspaceID = hex.EncodeToString(b)
		if err := r.SaveConfig(); err != nil {
			return "", err
		}
	}
	return r.Config.WorkspaceID, nil
}

// LocalEdits lists unsaved track-level changes in the working sets, and the
// modified sets themselves (stored in the local object store).
func (r *Repo) LocalEdits() ([]TrackEdit, []FileEntry, error) {
	ix := r.loadIndex()
	files, err := r.workingFiles(ix)
	if err != nil {
		return nil, nil, err
	}
	defer ix.save()
	head := map[string]FileEntry{}
	if id := r.Head(); id != "" {
		m, err := r.Load(id)
		if err != nil {
			return nil, nil, err
		}
		head = m.FileMap()
	}
	var edits []TrackEdit
	var changed []FileEntry
	for _, f := range files {
		if !isSet(f.Path) {
			continue
		}
		old, ok := head[f.Path]
		if ok && old.Hash == f.Hash {
			continue
		}
		if !r.Store.Has(f.Hash) {
			if _, _, err := r.Store.PutFile(r.Abs(f.Path)); err != nil {
				return nil, nil, err
			}
		}
		changed = append(changed, f)
		if !ok {
			edits = append(edits, TrackEdit{Set: f.Path, Name: "(new set)", Change: "added"})
			continue
		}
		d := r.workingSetDiff(old.Hash, f.Path)
		if d == nil {
			continue
		}
		for _, g := range d.GlobalChanges {
			edits = append(edits, TrackEdit{Set: f.Path, Name: g, Change: "modified"})
		}
		for _, tc := range d.TrackChanges {
			edits = append(edits, TrackEdit{Set: f.Path, TrackID: tc.TrackID, Name: tc.Name, Change: tc.Status})
		}
	}
	return edits, changed, nil
}

func (r *Repo) workingSetDiff(oldHash, rel string) *diff.SetDiff {
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
	return diff.Diff(old, cur)
}

// ReportWorkspace uploads this workspace's unsaved work to the server.
func (r *Repo) ReportWorkspace() (*WorkspaceState, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	id, err := r.WorkspaceID()
	if err != nil {
		return nil, err
	}
	edits, files, err := r.LocalEdits()
	if err != nil {
		return nil, err
	}
	var hashes []string
	for _, f := range files {
		hashes = append(hashes, f.Hash)
	}
	if err := r.uploadObjects(c, hashes); err != nil {
		return nil, err
	}
	authorID, author := r.Identity()
	st := &WorkspaceState{ID: id, Author: author, AuthorID: authorID, Branch: r.BranchName(), Base: r.Head(),
		Updated: time.Now().UTC().Format(time.RFC3339), Edits: edits, Files: files}
	if err := c.PutProject(remote.Project{ID: r.Config.ProjectID, Name: r.Config.Name}); err != nil {
		return nil, err
	}
	return st, c.PutWorkspace(r.Config.ProjectID, id, st)
}

// Teammates returns other members' workspaces that have unsaved edits.
func (r *Repo) Teammates() ([]WorkspaceState, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	var all []WorkspaceState
	if err := c.Workspaces(r.Config.ProjectID, &all); err != nil {
		if errors.Is(err, remote.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var out []WorkspaceState
	for _, w := range all {
		if w.ID != r.Config.WorkspaceID && len(w.Edits) > 0 {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Author < out[j].Author })
	return out, nil
}

// Overlaps describes tracks both you and a teammate are editing: the soft
// lock warning.
func Overlaps(mine []TrackEdit, others []WorkspaceState) []string {
	type key struct{ set, track string }
	editing := map[key]TrackEdit{}
	for _, e := range mine {
		if e.TrackID != "" && e.Change != "added" {
			editing[key{e.Set, e.TrackID}] = e
		}
	}
	var out []string
	for _, w := range others {
		for _, e := range w.Edits {
			if mine, ok := editing[key{e.Set, e.TrackID}]; ok && e.TrackID != "" && e.Change != "added" {
				out = append(out, fmt.Sprintf("you and %s are both editing %q in %s", w.Author, mine.Name, e.Set))
			}
		}
	}
	return out
}

// IncomingVersions lists versions on the server branch that this workspace
// does not have, newest first.
func (r *Repo) IncomingVersions() ([]*Manifest, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if errors.Is(err, remote.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	target := heads[r.BranchName()]
	if target == "" || target == r.Latest() {
		return nil, nil
	}
	if err := r.fetchSnapshots(c, target); err != nil {
		return nil, err
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
