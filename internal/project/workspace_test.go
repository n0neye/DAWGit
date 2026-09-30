package project

import (
	"path/filepath"
	"testing"
)

func editNames(edits []TrackEdit) map[string]string {
	out := map[string]string{}
	for _, e := range edits {
		out[e.TrackID+" "+e.Name] = e.Change
	}
	return out
}

func TestLocalEditsAndIncoming(t *testing.T) {
	a, b := team(t)

	// Yi edits in Live without committing: the tracks show as Yi's changes.
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	edits, err := a.LocalEdits()
	if err != nil {
		t.Fatal(err)
	}
	names := editNames(edits)
	if names["16 Audios"] != "added" || names["14 6 Bounce + Reverb"] != "modified" {
		t.Fatalf("A's edits: %v", names)
	}
	if in, err := b.IncomingVersions(); err != nil || len(in) != 0 {
		t.Fatalf("nothing committed yet, but incoming: %v %+v", err, in)
	}

	// Yi commits: Alex is told about the version; nothing changes locally.
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	if _, _, err := a.Save("group audio", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if edits, _ := a.LocalEdits(); len(edits) != 0 {
		t.Errorf("after commit: %+v", edits)
	}
	incoming, err := b.IncomingVersions()
	if err != nil || len(incoming) != 1 || incoming[0].Message != "group audio" || incoming[0].Author != "yi" {
		t.Fatalf("incoming: %v %+v", err, incoming)
	}
	if edits, _ := b.LocalEdits(); len(edits) == 0 {
		t.Error("B's unsaved work disappeared")
	}
}
