package project

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesHistoryAndRestore(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	os.WriteFile(filepath.Join(root, "Samples", "kick.wav"), []byte("RIFF-v1"), 0o644)
	v1 := mustSnapshot(t, r, "v1")
	os.WriteFile(filepath.Join(root, "Samples", "kick.wav"), []byte("RIFF-v2"), 0o644)
	v2 := mustSnapshot(t, r, "v2")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(root, "Song.als"))
	mustSnapshot(t, r, "v3 (set only)")

	// Changed files only, then everything (Live's backups show as ignored).
	os.WriteFile(filepath.Join(root, "Samples", "kick.wav"), []byte("RIFF-now"), 0o644)
	changed, err := r.Files(false)
	if err != nil || len(changed) != 1 || changed[0].Path != "Samples/kick.wav" || changed[0].Status != "modified" {
		t.Fatalf("changed files: %v %+v", err, changed)
	}
	all, _ := r.Files(true)
	st := map[string]string{}
	for _, f := range all {
		st[f.Path] = f.Status
	}
	if st["Song.als"] != "unchanged" || st["Backup/Song [old].als"] != "ignored" || st["Samples/kick.wav"] != "modified" {
		t.Fatalf("all files: %v", st)
	}

	hist, err := r.FileHistory("Samples/kick.wav")
	if err != nil || len(hist) != 2 || hist[0].Version.ID != v2.ID || hist[0].Status != "modified" ||
		hist[1].Version.ID != v1.ID || hist[1].Status != "added" {
		t.Fatalf("history: %v %+v", err, hist)
	}
	f, err := r.OpenFile("Samples/kick.wav", v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(f)
	f.Close()
	if string(data) != "RIFF-v1" {
		t.Fatalf("v1 content %q", data)
	}

	// Discard: back to the version the project is on.
	if err := r.RestoreFile("Samples/kick.wav", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "Samples", "kick.wav")); string(got) != "RIFF-v2" {
		t.Fatalf("discarded content %q", got)
	}
	assertClean(t, r)
	// A file the version does not have is removed.
	os.WriteFile(filepath.Join(root, "Samples", "new.wav"), []byte("RIFF"), 0o644)
	r.RestoreFile("Samples/new.wav", "")
	if _, err := os.Stat(filepath.Join(root, "Samples", "new.wav")); !os.IsNotExist(err) {
		t.Error("added file not removed")
	}
	if d, err := r.FileDiff("Song.als", v2.ID, r.Head()); err != nil || d == nil || d.Empty() {
		t.Errorf("set diff between versions: %v %v", err, d)
	}
}
