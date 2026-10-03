package desktop

import (
	"errors"
	"io"
	"io/fs"

	"dawgit/internal/als"
	"dawgit/internal/diff"
	"dawgit/internal/project"
)

// SetView is a Live Set drawn as Live shows it, and how its tracks changed
// from another version of it.
type SetView struct {
	Now    *als.Overview `json:"now"`    // nil: no set in that version
	Before *als.Overview `json:"before"` // nil: nothing to compare with
	Global []string      `json:"global"` // tempo, main track, ...
	// GlobalWeights weigh Global (diff.Weight*); Weight is the heaviest
	// change of all.
	GlobalWeights []string         `json:"globalWeights"`
	Weight        string           `json:"weight"`
	Order         bool             `json:"order"` // the tracks' order changed
	Changes       []SetTrackChange `json:"changes"`
	Text          []string         `json:"text"` // the changes as text lines
}

type SetTrackChange struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"` // added | removed | modified
	Details []string `json:"details"`
	Weight  string   `json:"weight"`  // the track's heaviest change
	Weights []string `json:"weights"` // each detail's
}

// SetOverview reads a set in a version ("" for the project folder now,
// "none" for no set) and, unless fromVersion is "none", compares it with
// the set in fromVersion (at fromFile when it was elsewhere, "" for file).
func (a *App) SetOverview(root, file, version, fromFile, fromVersion string) (*SetView, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if fromFile == "" {
		fromFile = file
	}
	now, err := readSet(r, file, version)
	if err != nil {
		return nil, err
	}
	before, err := readSet(r, fromFile, fromVersion)
	if err != nil {
		return nil, err
	}
	out := &SetView{Global: []string{}, GlobalWeights: []string{}, Changes: []SetTrackChange{}, Text: []string{}}
	if now != nil {
		out.Now = now.Overview()
	}
	if before != nil {
		out.Before = before.Overview()
	}
	if now != nil && before != nil {
		d := diff.Diff(before, now)
		out.Global = nonNil(d.GlobalChanges)
		out.Order = d.OrderChanged
		out.Text = diffLines(d)
		out.Weight = d.Weight()
		for _, g := range out.Global {
			out.GlobalWeights = append(out.GlobalWeights, diff.GlobalWeight(g))
		}
		for _, tc := range d.TrackChanges {
			ws := []string{}
			for _, x := range tc.Details {
				ws = append(ws, diff.DetailWeight(x))
			}
			out.Changes = append(out.Changes, SetTrackChange{ID: tc.TrackID, Status: tc.Status, Details: nonNil(tc.Details),
				Weight: tc.Weight(), Weights: ws})
		}
	}
	return out, nil
}

// readSet loads a set as it is in a version; nil when there is none.
func readSet(r *project.Repo, file, version string) (*als.LiveSet, error) {
	if version == "none" {
		return nil, nil
	}
	f, err := r.OpenFile(file, version)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return als.FromGzip(data)
}
