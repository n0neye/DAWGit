package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Chinese and emoji names, a leading space, and paths over 260 characters
// go through a version and back.
func TestUnusualNamesAndLongPaths(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	long := filepath.Join(root, strings.Repeat("很長的資料夾名稱abcdefghij", 6), strings.Repeat("Sub Folder ", 8)+"end")
	names := []string{
		filepath.Join(root, "鼓組", "大鼓 Kick (最終版).wav"),
		filepath.Join(root, " leading space.wav"),
		filepath.Join(root, "emoji 🎵.txt"),
		filepath.Join(long, "deep file.wav"),
	}
	for i, p := range names {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte{byte(i)}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("longest path: %d chars", len(names[3]))
	a := mustSnapshot(t, r, "names")
	if len(a.Files) < 4 {
		t.Fatalf("%d files", len(a.Files))
	}
	for _, p := range names {
		os.Remove(p)
	}
	b := mustSnapshot(t, r, "gone")
	if _, _, err := r.GoTo(a.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range names {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("not back: %v", err)
		}
	}
	assertClean(t, r)
	if _, _, err := r.GoTo(b.ID, false); err != nil {
		t.Fatal(err)
	}
	assertClean(t, r)
}

// Committing some changes leaves the others uncommitted.
func TestCommitSomeChanges(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(root, "gone.txt"), []byte("x"), 0o644)
	first := mustSnapshot(t, r, "first")

	os.WriteFile(filepath.Join(root, "a.txt"), []byte("a2"), 0o644)
	os.WriteFile(filepath.Join(root, "new.txt"), []byte("n"), 0o644)
	os.WriteFile(filepath.Join(root, "later.txt"), []byte("l"), 0o644)
	os.Remove(filepath.Join(root, "gone.txt"))
	r.Only = []string{"a.txt", "new.txt", "gone.txt"}
	m := mustSnapshot(t, r, "some")
	r.Only = nil
	fm := m.FileMap()
	if _, ok := fm["later.txt"]; ok {
		t.Fatal("later.txt wasn't picked")
	}
	if _, ok := fm["gone.txt"]; ok {
		t.Fatal("gone.txt was picked: it should be gone")
	}
	if fm["a.txt"].Hash == first.FileMap()["a.txt"].Hash || fm["new.txt"].Hash == "" {
		t.Fatal("a.txt and new.txt should be committed")
	}
	changes, _ := r.Status()
	if len(changes) != 1 || changes[0].Path != "later.txt" || changes[0].Status != "added" {
		t.Fatalf("left uncommitted: %+v", changes)
	}
	// Nothing picked that changed: nothing to commit.
	r.Only = []string{"a.txt"}
	if _, err := r.Snapshot("again"); err != ErrNothingToSnapshot {
		t.Fatalf("nothing picked: %v", err)
	}
}
