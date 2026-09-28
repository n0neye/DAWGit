package project

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/als"
)

var fixtureProject = filepath.Join("..", "..", "SampleProjects", "SampleAbletonProject Project")

// newProject copies the v2 fixture (without backups) into a temp folder.
func newProject(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "Song Project")
	copyFile(t, filepath.Join(fixtureProject, "SampleAbletonProject_v2.als"), filepath.Join(dst, "Song.als"))
	copyTree(t, filepath.Join(fixtureProject, "Samples"), filepath.Join(dst, "Samples"))
	copyTree(t, filepath.Join(fixtureProject, "Ableton Project Info"), filepath.Join(dst, "Ableton Project Info"))
	// Live litter that must be ignored.
	os.MkdirAll(filepath.Join(dst, "Backup"), 0o755)
	os.WriteFile(filepath.Join(dst, "Backup", "Song [old].als"), []byte("x"), 0o644)
	return dst
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		copyFile(t, p, filepath.Join(dst, rel))
		return nil
	})
}

func mustSnapshot(t *testing.T, r *Repo, msg string) *Manifest {
	t.Helper()
	m, err := r.Snapshot(msg)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func assertClean(t *testing.T, r *Repo) {
	t.Helper()
	changes, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range changes {
		t.Errorf("unexpected change: %s %s", c.Status, c.Path)
	}
}

func TestSnapshotStatusLog(t *testing.T) {
	root := newProject(t)
	r, err := Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	m1 := mustSnapshot(t, r, "first")
	for _, f := range m1.Files {
		if strings.HasPrefix(f.Path, "Backup/") || strings.HasSuffix(f.Path, ".asd") {
			t.Errorf("ignored file in snapshot: %s", f.Path)
		}
	}
	if len(m1.Packs) != 1 || m1.Packs[0] != "Core Library" {
		t.Errorf("packs = %v", m1.Packs)
	}
	assertClean(t, r)
	if _, err := r.Snapshot("again"); !errors.Is(err, ErrNothingToSnapshot) {
		t.Errorf("expected ErrNothingToSnapshot, got %v", err)
	}

	// Simulate an edit in Live: the set becomes Split-A's content.
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(root, "Song.als"))
	changes, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Status != "modified" || changes[0].SetDiff == nil {
		t.Fatalf("changes = %+v", changes)
	}
	if !strings.Contains(changes[0].SetDiff.Render(), `+ GroupTrack "Audios"`) {
		t.Errorf("set diff:\n%s", changes[0].SetDiff.Render())
	}
	m2 := mustSnapshot(t, r, "group audio")
	log, err := r.Log()
	if err != nil || len(log) != 2 || log[0].ID != m2.ID || log[1].ID != m1.ID {
		t.Fatalf("log = %v %v", log, err)
	}

	// Reopen from a subfolder.
	r2, err := Open(filepath.Join(root, "Samples"))
	if err != nil || r2.Root != r.Root || r2.Config.Author != "yi" {
		t.Fatalf("open: %v %+v", err, r2)
	}
}

func TestCheckout(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	m1 := mustSnapshot(t, r, "v2")
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(root, "Song.als"))
	os.WriteFile(filepath.Join(root, "Samples", "new.wav"), []byte("RIFF"), 0o644)
	mustSnapshot(t, r, "split b")

	// Uncommitted edits block checkout.
	os.WriteFile(filepath.Join(root, "Samples", "new.wav"), []byte("RIFF2"), 0o644)
	if _, _, err := r.Checkout(m1.ID[:8], false); !errors.Is(err, ErrDirty) {
		t.Fatalf("expected ErrDirty, got %v", err)
	}
	if _, _, err := r.Checkout(m1.ID[:8], true); err != nil {
		t.Fatal(err)
	}
	if r.Head() != m1.ID {
		t.Error("HEAD not moved")
	}
	if _, err := os.Stat(filepath.Join(root, "Samples", "new.wav")); !os.IsNotExist(err) {
		t.Error("file added later should be removed")
	}
	s, err := als.Load(filepath.Join(root, "Song.als"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.TrackByID()["16"]; ok {
		t.Error("track from the later snapshot still present")
	}
	assertClean(t, r)
}

// Moving the project (another machine, another folder) relinks project
// samples to the new location without showing the set as modified.
func TestCheckoutRelinksProjectSamplesAfterMove(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	m := mustSnapshot(t, r, "v2")

	moved := filepath.Join(t.TempDir(), "Elsewhere", "Song Project")
	copyTree(t, root, moved)
	r2, err := Open(moved)
	if err != nil {
		t.Fatal(err)
	}
	_, notes, err := r2.Checkout("HEAD", true)
	if err != nil {
		t.Fatal(err)
	}
	// Preset/device source paths from other machines must not be touched.
	for _, n := range notes {
		t.Errorf("unexpected relink note: %s", n)
	}
	s, _ := als.Load(filepath.Join(moved, "Song.als"))
	for _, ref := range s.SampleRefs() {
		if ref.RelativePathType == "3" && !strings.HasPrefix(ref.Path, filepath.ToSlash(moved)) {
			t.Errorf("not relinked: %s", ref.Path)
		}
	}
	assertClean(t, r2)
	if r2.Head() != m.ID {
		t.Error("HEAD changed")
	}
}

// A sample outside the project is stored with the snapshot and materialized
// on a machine that does not have it.
func TestExternalSampleRoundTrip(t *testing.T) {
	root := newProject(t)
	extDir := filepath.Join(t.TempDir(), "My Samples")
	ext := filepath.Join(extDir, "kick.wav")
	os.MkdirAll(extDir, 0o755)
	os.WriteFile(ext, []byte("RIFF-kick"), 0o644)

	// Point the bounce clip's sample at the external file, as Live does for
	// samples dragged in from outside the project without collecting.
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

	r, _ := Init(root, "yi")
	m := mustSnapshot(t, r, "with external")
	if len(m.External) != 1 || m.External[0].Path != filepath.ToSlash(ext) {
		t.Fatalf("external = %+v", m.External)
	}

	// Another machine: project copied, external file absent.
	other := filepath.Join(t.TempDir(), "Song Project")
	copyTree(t, root, other)
	os.Remove(ext)
	r2, _ := Open(other)
	_, notes, err := r2.Checkout("HEAD", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) == 0 {
		t.Fatal("expected a relink note")
	}
	s2, _ := als.Load(filepath.Join(other, "Song.als"))
	var relinked string
	for _, ref := range s2.SampleRefs() {
		if strings.HasSuffix(ref.Path, "/kick.wav") {
			relinked = ref.Path
		}
	}
	data, err := os.ReadFile(filepath.FromSlash(relinked))
	if err != nil || string(data) != "RIFF-kick" {
		t.Fatalf("relinked sample %q: %v %q", relinked, err, data)
	}
	for _, ref := range s2.SampleRefs() {
		if ref.Path == relinked && !strings.HasPrefix(ref.RelativePath, ".dawgit/external/") {
			t.Errorf("relative path not updated: %s", ref.RelativePath)
		}
	}
	assertClean(t, r2)

	// Saved again in Live on that machine; Live may record the cached file
	// as project-relative. The snapshot still records the original path.
	for _, sr := range s2.Root.Iter("SampleRef") {
		if fr := sr.Child("FileRef"); strings.HasSuffix(fr.Val("Path", ""), "/kick.wav") {
			fr.Find("RelativePathType").Set("Value", "3")
		}
	}
	s2.Save(filepath.Join(other, "Song.als"))
	os.WriteFile(filepath.Join(other, "Samples", "touch.txt"), []byte("x"), 0o644)
	m2 := mustSnapshot(t, r2, "resaved")
	if len(m2.External) != 1 || m2.External[0].Path != filepath.ToSlash(ext) {
		t.Fatalf("external after resave = %+v", m2.External)
	}

	// A third machine gets a set pointing into the second machine's cache.
	third := filepath.Join(t.TempDir(), "Song Project")
	copyTree(t, other, third)
	os.RemoveAll(filepath.Join(third, ".dawgit", "external"))
	r3, _ := Open(third)
	if _, _, err := r3.Checkout("HEAD", true); err != nil {
		t.Fatal(err)
	}
	s3, _ := als.Load(filepath.Join(third, "Song.als"))
	for _, ref := range s3.SampleRefs() {
		if strings.HasSuffix(ref.Path, "/kick.wav") {
			if !strings.HasPrefix(ref.Path, filepath.ToSlash(third)) {
				t.Errorf("not relinked into this machine's cache: %s", ref.Path)
			}
			if data, err := os.ReadFile(filepath.FromSlash(ref.Path)); err != nil || string(data) != "RIFF-kick" {
				t.Errorf("sample not materialized: %v", err)
			}
		}
	}
	assertClean(t, r3)
}
