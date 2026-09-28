package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func editNames(edits []TrackEdit) map[string]string {
	out := map[string]string{}
	for _, e := range edits {
		out[e.TrackID+" "+e.Name] = e.Change
	}
	return out
}

func TestSoftLocksAndNotifications(t *testing.T) {
	a, b := team(t)

	// Both edit in Live without saving a version.
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))

	aState, err := a.ReportWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	names := editNames(aState.Edits)
	if names["16 Audios"] != "added" || names["14 6 Bounce + Reverb"] != "modified" {
		t.Fatalf("A's edits: %v", names)
	}
	if len(aState.Files) != 1 || aState.Files[0].Path != "Song.als" {
		t.Errorf("A's backup files: %+v", aState.Files)
	}
	bState, err := b.ReportWorkspace()
	if err != nil {
		t.Fatal(err)
	}

	// Alex sees what Yi is editing, and the overlap.
	mates, err := b.Teammates()
	if err != nil || len(mates) != 1 || mates[0].Author != "yi" {
		t.Fatalf("teammates: %v %+v", err, mates)
	}
	overlaps := Overlaps(bState.Edits, mates)
	joined := strings.Join(overlaps, "\n")
	if len(overlaps) != 2 || !strings.Contains(joined, "you and yi are both editing") ||
		!strings.Contains(joined, "Bounce + Reverb") || !strings.Contains(joined, "80s Beat") {
		t.Fatalf("overlaps: %v", overlaps)
	}
	// New tracks created by both (same id 16) are not a lock.
	if strings.Contains(joined, "Drum") || strings.Contains(joined, "Audios") {
		t.Errorf("new tracks reported as overlap: %v", overlaps)
	}

	// Yi saves: the soft lock disappears, Alex is told about the version.
	if _, _, err := a.Save("group audio", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if st, err := a.ReportWorkspace(); err != nil || len(st.Edits) != 0 {
		t.Fatalf("after save: %v %+v", err, st)
	}
	if mates, _ := b.Teammates(); len(mates) != 0 {
		t.Errorf("stale soft lock: %+v", mates)
	}
	incoming, err := b.IncomingVersions()
	if err != nil || len(incoming) != 1 || incoming[0].Message != "group audio" || incoming[0].Author != "yi" {
		t.Fatalf("incoming: %v %+v", err, incoming)
	}
	// Notifying changed nothing locally.
	if edits, _, _ := b.LocalEdits(); len(edits) == 0 {
		t.Error("B's unsaved work disappeared")
	}
}
