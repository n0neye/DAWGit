package project

import (
	"os"
	"path/filepath"
	"testing"
)

// On Windows "Kick.wav" and "kick.wav" are the same file: going between
// versions that differ only in a name's case keeps the file.
func TestCaseOnlyRenameSurvivesCheckout(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	os.WriteFile(filepath.Join(root, "Kick.wav"), []byte("kick"), 0o644)
	a := mustSnapshot(t, r, "upper")
	os.Rename(filepath.Join(root, "Kick.wav"), filepath.Join(root, "kick-tmp"))
	os.Rename(filepath.Join(root, "kick-tmp"), filepath.Join(root, "kick.wav"))
	b := mustSnapshot(t, r, "lower")
	if _, _, err := r.GoTo(a.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Kick.wav")); err != nil {
		t.Errorf("after going back: %v", err)
	}
	if _, _, err := r.GoTo(b.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "kick.wav")); err != nil {
		t.Errorf("after going forward: %v", err)
	}
}
