package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// Projects from Perforce have read-only files: going between versions and
// restoring a file still work.
func TestReadOnlyFilesDontStopCheckout(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	p := filepath.Join(root, "Kick.wav")
	os.WriteFile(p, []byte("kick"), 0o644)
	a := mustSnapshot(t, r, "a")
	os.Chmod(p, 0o644)
	os.WriteFile(p, []byte("kick 2"), 0o644)
	os.WriteFile(filepath.Join(root, "Snare.wav"), []byte("snare"), 0o644)
	b := mustSnapshot(t, r, "b")
	os.Chmod(p, 0o444)
	os.Chmod(filepath.Join(root, "Snare.wav"), 0o444)
	if _, _, err := r.GoTo(a.ID, false); err != nil {
		t.Fatalf("back with read-only files: %v", err)
	}
	if _, _, err := r.GoTo(b.ID, false); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if err := r.RestoreFile("Kick.wav", a.ID); err != nil {
		t.Fatalf("restore over read-only: %v", err)
	}
}

// Only one program changes a project at a time.
func TestProjectLock(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	mustSnapshot(t, r, "first")
	unlock, err := r.Lock(0)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := Open(root)
	if _, err := other.Lock(200 * time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock: %v", err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		unlock()
	}()
	release, err := other.Lock(2 * time.Second)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	release()
	assertClean(t, r) // the lock file isn't a project file
}
