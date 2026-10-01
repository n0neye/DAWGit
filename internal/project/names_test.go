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
