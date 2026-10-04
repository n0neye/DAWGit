package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/als"
)

// pointBounceAt makes the set's bounce clip use an external file, as Live
// does for a sample dragged in from outside the project.
func pointBounceAt(t *testing.T, root, ext string) {
	t.Helper()
	s, _ := als.Load(filepath.Join(root, "Song.als"))
	for _, fr := range s.Root.Iter("FileRef") {
		if strings.Contains(fr.Val("RelativePath", ""), "Bounce") {
			fr.Find("RelativePathType").Set("Value", "1")
			rel, _ := filepath.Rel(root, ext)
			fr.Find("RelativePath").Set("Value", filepath.ToSlash(rel))
			fr.Find("Path").Set("Value", filepath.ToSlash(ext))
		}
	}
	s.Save(filepath.Join(root, "Song.als"))
}

func refTo(t *testing.T, root, name string) als.SampleRef {
	t.Helper()
	s, _ := als.Load(filepath.Join(root, "Song.als"))
	for _, ref := range s.SampleRefs() {
		if strings.HasSuffix(ref.Path, "/"+name) {
			return ref
		}
	}
	t.Fatalf("no reference to %s", name)
	return als.SampleRef{}
}

// The external drive is gone: the sample comes back from the version that
// kept it, into Samples/Imported, and the set points there.
func TestRestoreMissingExternalSample(t *testing.T) {
	root := newProject(t)
	extDir := filepath.Join(t.TempDir(), "Drive", "My Samples")
	ext := filepath.Join(extDir, "kick.wav")
	os.MkdirAll(extDir, 0o755)
	os.WriteFile(ext, []byte("RIFF-kick"), 0o644)
	pointBounceAt(t, root, ext)
	r, _ := Init(root, "yi")
	mustSnapshot(t, r, "with external")
	// Pruned from .dawgit as after sharing would make no difference here:
	// the store has it.
	os.RemoveAll(filepath.Join(t.TempDir(), "Drive"))
	os.RemoveAll(extDir)

	spots, err := r.SampleSpots()
	if err != nil || len(spots) != 1 || !spots[0].Restorable() || spots[0].Name != "kick.wav" {
		t.Fatalf("spots %+v %v", spots, err)
	}
	n, err := r.BringSamplesIn(true, false)
	if err != nil || n != 1 {
		t.Fatalf("brought %d: %v", n, err)
	}
	got, err := os.ReadFile(filepath.Join(root, "Samples", "Imported", "kick.wav"))
	if err != nil || string(got) != "RIFF-kick" {
		t.Fatalf("restored %q %v", got, err)
	}
	ref := refTo(t, root, "kick.wav")
	if ref.RelativePathType != "3" || ref.RelativePath != "Samples/Imported/kick.wav" {
		t.Errorf("set points at %+v", ref)
	}
	if spots, _ := r.SampleSpots(); len(spots) != 0 {
		t.Errorf("still to fix: %+v", spots)
	}
	// It shows as changes, to commit.
	changes, _ := r.Status()
	var paths []string
	for _, c := range changes {
		paths = append(paths, c.Status+" "+c.Path)
	}
	if !strings.Contains(strings.Join(paths, ","), "added Samples/Imported/kick.wav") ||
		!strings.Contains(strings.Join(paths, ","), "modified Song.als") {
		t.Errorf("changes %v", paths)
	}
}

// A teammate's sample is only in .dawgit: brought into the project before
// .dawgit goes (unlinking, leaving the team), so the project works without it.
func TestBringKeptSamplesIn(t *testing.T) {
	root := newProject(t)
	ext := filepath.Join(t.TempDir(), "Lib", "snare.wav")
	os.MkdirAll(filepath.Dir(ext), 0o755)
	os.WriteFile(ext, []byte("RIFF-snare"), 0o644)
	pointBounceAt(t, root, ext)
	r, _ := Init(root, "yi")
	mustSnapshot(t, r, "with external")
	other := filepath.Join(t.TempDir(), "B", "Song Project")
	copyTree(t, root, other)
	os.Remove(ext)
	r2, _ := Open(other)
	if _, _, err := r2.Checkout("HEAD", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(refTo(t, other, "snare.wav").Path, "/.dawgit/external/") {
		t.Fatal("expected the sample in .dawgit")
	}
	spots, _ := r2.SampleSpots()
	if len(spots) != 1 || !spots[0].Kept || spots[0].Missing {
		t.Fatalf("spots %+v", spots)
	}
	if n, err := r2.BringSamplesIn(false, true); err != nil || n != 1 {
		t.Fatalf("brought %d: %v", n, err)
	}
	os.RemoveAll(filepath.Join(other, ".dawgit")) // the project works without it
	ref := refTo(t, other, "snare.wav")
	got, err := os.ReadFile(filepath.FromSlash(ref.Path))
	if err != nil || string(got) != "RIFF-snare" || ref.RelativePath != "Samples/Imported/snare.wav" {
		t.Fatalf("after: %+v %q %v", ref, got, err)
	}
}
