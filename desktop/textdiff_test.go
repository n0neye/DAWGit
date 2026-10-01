package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
)

func TestTextDiff(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	t.Cleanup(waitTidy)
	a := NewApp()
	root := newSong(t)
	notes := filepath.Join(root, "notes.txt")
	os.WriteFile(notes, []byte("intro\nverse\nchorus\n"), 0o644)
	if _, err := a.AddLocalProject(root); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(root, "first", true, nil, true); err != nil {
		t.Fatal(err)
	}
	r, _ := project.Open(root)
	head := r.Head()

	os.WriteFile(notes, []byte("intro\nverse 2\nchorus\noutro\n"), 0o644)
	d, err := a.TextDiff(root, "notes.txt", head, "")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Text || d.Added != 2 || d.Removed != 1 || len(d.Hunks) != 1 {
		t.Fatalf("now vs committed: %+v", d)
	}

	// Added in a version: everything is new.
	d, err = a.TextDiff(root, "notes.txt", "none", head)
	if err != nil || d.Added != 3 || d.Removed != 0 {
		t.Fatalf("added: %+v %v", d, err)
	}
	// Deleted from the folder.
	os.Remove(notes)
	if d, err = a.TextDiff(root, "notes.txt", head, ""); err != nil || d.Removed != 3 {
		t.Fatalf("deleted: %+v %v", d, err)
	}
	// Not text.
	if d, err = a.TextDiff(root, "Song.als", head, ""); err != nil || d.Text {
		t.Fatalf("a set isn't text: %+v %v", d, err)
	}
}
