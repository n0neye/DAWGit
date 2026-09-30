package project

import (
	"path/filepath"
	"testing"
)

// A changed set is parsed once; status and backups reuse the diff until the
// set changes again.
func TestSetDiffCached(t *testing.T) {
	root := newProject(t)
	r, err := Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	mustSnapshot(t, r, "first")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(root, "Song.als"))

	first, _ := r.Status()
	again, _ := r.Status()
	if len(first) != 1 || first[0].SetDiff == nil || again[0].SetDiff != first[0].SetDiff {
		t.Fatalf("second status parsed the set again: %+v %+v", first, again)
	}
	if _, _, err := r.LocalEdits(); err != nil {
		t.Fatal(err)
	}

	// Saved again with other content: a new diff.
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(root, "Song.als"))
	later, _ := r.Status()
	if len(later) != 1 || later[0].SetDiff == nil || later[0].SetDiff == first[0].SetDiff {
		t.Fatalf("changed set kept the old diff")
	}
	if later[0].SetDiff.Render() == first[0].SetDiff.Render() {
		t.Errorf("diffs of different contents render the same:\n%s", later[0].SetDiff.Render())
	}
}
