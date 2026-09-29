package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/als"
)

func fileOf(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGoToOlderVersionAndBack(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	v1 := mustSnapshot(t, r, "v1")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(root, "Song.als"))
	v2 := mustSnapshot(t, r, "v2")

	if _, _, err := r.GoTo(v1.ID[:8], false); err != nil {
		t.Fatal(err)
	}
	if r.Head() != v1.ID {
		t.Fatal("not on v1")
	}
	assertClean(t, r) // the files are v1's (sets relinked for this computer)
	if !r.OnOlderVersion() || r.Latest() != v2.ID {
		t.Fatalf("older=%v latest=%s", r.OnOlderVersion(), r.Latest())
	}
	r2, _ := Open(root) // remembered
	if !r2.OnOlderVersion() {
		t.Fatal("older version not saved in the config")
	}
	if log, _ := r.Log(); len(log) != 2 {
		t.Fatalf("history on an older version has %d versions, want 2", len(log))
	}
	if _, err := r.Snapshot("x"); !errors.Is(err, ErrOlderVersion) {
		t.Fatalf("snapshot on an older version: %v", err)
	}

	// Changes made on the older version need discard to leave.
	os.WriteFile(filepath.Join(root, "Samples", "new.wav"), []byte("RIFF"), 0o644)
	if _, _, err := r.GoTo("latest", false); !errors.Is(err, ErrDirty) {
		t.Fatalf("leaving with changes: %v", err)
	}
	if _, _, err := r.GoTo("latest", true); err != nil {
		t.Fatal(err)
	}
	if r.OnOlderVersion() || r.Head() != v2.ID {
		t.Fatal("not back at the latest version")
	}
	assertClean(t, r)
}

func TestKeepThisVersion(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	v1 := mustSnapshot(t, r, "v1")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(root, "Song.als"))
	v2 := mustSnapshot(t, r, "v2")

	r.GoTo(v1.ID, false)
	v3, err := r.KeepThisVersion("back to v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.OnOlderVersion() || len(v3.Parents) != 1 || v3.Parents[0] != v2.ID {
		t.Fatalf("v3 = %+v", v3)
	}
	if v3.FileMap()["Song.als"] != v1.FileMap()["Song.als"] {
		t.Fatal("content of v1 not kept")
	}
	if log, _ := r.Log(); len(log) != 3 {
		t.Fatalf("history has %d versions, want 3", len(log))
	}
}

// On a team: an older version hides no incoming versions, cannot be shared
// on the branch, and a branch can grow from it.
func TestTeamBranchFromOlderVersion(t *testing.T) {
	a, b := team(t)
	v1 := a.Head()
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, _, err := a.Save("v2", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := b.GoTo(v1, false); err != nil {
		t.Fatal(err)
	}
	if in, _ := b.IncomingVersions(); len(in) != 0 {
		t.Fatalf("versions after the older one shown as incoming: %d", len(in))
	}
	if inc, _ := b.Incoming(); inc {
		t.Fatal("Incoming() on an older version")
	}
	if _, _, err := b.Save("x", Strategy("fail")); !errors.Is(err, ErrOlderVersion) {
		t.Fatalf("save on an older version: %v", err)
	}
	if _, err := b.Update(Strategy("fail")); !errors.Is(err, ErrOlderVersion) {
		t.Fatalf("update on an older version: %v", err)
	}

	if err := b.CreateBranch("alex-idea"); err != nil {
		t.Fatal(err)
	}
	if b.OnOlderVersion() || b.BranchName() != "alex-idea" {
		t.Fatal("branch did not continue from the older version")
	}
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	m, res, err := b.Save("idea", Strategy("fail"))
	if err != nil || res.Action != "published" || m.Parents[0] != v1 {
		t.Fatalf("save on the new branch: %v %+v %+v", err, res, m)
	}
}

// Going to a version whose files were never downloaded fetches them.
func TestGoToFetchesMissingFiles(t *testing.T) {
	a, b := team(t)
	v1 := a.Head()
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	a.Save("v2", Strategy("fail"))
	b.Update(Strategy("fail"))
	m1, _ := b.Load(v1)
	for _, h := range m1.Objects() {
		os.Remove(b.Store.Path(h))
	}
	if _, _, err := b.GoTo(v1, false); err != nil {
		t.Fatal(err)
	}
	assertClean(t, b)
}

func TestExportVersion(t *testing.T) {
	root := newProject(t)
	extDir := filepath.Join(t.TempDir(), "My Samples")
	ext := filepath.Join(extDir, "kick.wav")
	os.MkdirAll(extDir, 0o755)
	os.WriteFile(ext, []byte("RIFF-kick"), 0o644)
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
	v1 := mustSnapshot(t, r, "with external")

	dir := filepath.Join(t.TempDir(), r.ExportName(v1))
	if _, err := r.Export(v1.ID, dir); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "Song (") {
		t.Errorf("export name %q", filepath.Base(dir))
	}
	if _, err := os.Stat(filepath.Join(dir, ".dawgit")); err == nil {
		t.Error("export contains .dawgit")
	}
	if got := fileOf(t, filepath.Join(dir, "Samples", "Imported", "kick.wav")); got != "RIFF-kick" {
		t.Fatalf("imported sample = %q", got)
	}
	s2, _ := als.Load(filepath.Join(dir, "Song.als"))
	dirSlash := filepath.ToSlash(dir)
	for _, ref := range s2.SampleRefs() {
		if ref.Pack != "" || ref.Path == "" {
			continue
		}
		if !strings.HasPrefix(ref.Path, dirSlash+"/") {
			t.Errorf("sample points outside the copy: %s", ref.Path)
		}
	}
	if _, err := r.Export(v1.ID, dir); err == nil {
		t.Error("export into a non-empty folder")
	}
	assertClean(t, r) // the project itself is untouched
}

// Going to the latest version of another branch puts the project on that
// branch: commits go there directly.
func TestGoToAnotherBranchLatest(t *testing.T) {
	a, _ := team(t)
	mainHead := a.Head()
	a.CreateBranch("idea")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, _, err := a.Save("idea 1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}

	if _, _, err := a.GoTo(mainHead, false); err != nil {
		t.Fatal(err)
	}
	if a.OnOlderVersion() || a.BranchName() != "main" {
		t.Fatalf("older=%v branch=%s", a.OnOlderVersion(), a.BranchName())
	}
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(a.Root, "Song.als"))
	m, res, err := a.Save("on main", Strategy("fail"))
	if err != nil || res.Action != "published" || m.Parents[0] != mainHead {
		t.Fatalf("commit on main: %v %+v", err, res)
	}

	// Back on idea's older version (not a branch's latest): still "older".
	a.GoTo(mainHead, false)
	if !a.OnOlderVersion() {
		t.Fatal("a version that is no branch's latest should count as older")
	}
}

// Versions not shared yet keep the project where it is.
func TestAdoptKeepsUnsharedVersions(t *testing.T) {
	a, _ := team(t)
	mainHead := a.Head()
	a.CreateBranch("idea")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	mustSnapshot(t, a, "not shared")
	if _, _, err := a.GoTo(mainHead, false); err != nil {
		t.Fatal(err)
	}
	if !a.OnOlderVersion() || a.BranchName() != "idea" {
		t.Fatalf("older=%v branch=%s", a.OnOlderVersion(), a.BranchName())
	}
}

// Changes made on an older version can be committed after it and combined
// with the latest version, like teammates' work.
func TestCommitOnOlderVersionThenCombine(t *testing.T) {
	a, b := team(t)
	v1 := a.Head()
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	v2, _, err := a.Save("v2", Strategy("fail"))
	if err != nil {
		t.Fatal(err)
	}
	b.Update(Strategy("fail"))
	if _, _, err := b.GoTo(v1, false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(b.Root, "Samples", "older-idea.wav"), []byte("RIFF-idea"), 0o644)
	m, err := b.CommitOnOlderVersion("idea on v1")
	if err != nil || m.Parents[0] != v1 || b.OnOlderVersion() {
		t.Fatalf("commit on older: %v %+v older=%v", err, m, b.OnOlderVersion())
	}
	_, res, err := b.Save("idea on v1", Strategy("fail"))
	if err != nil || res.Action != "published" {
		t.Fatalf("combine: %v %+v", err, res)
	}
	head, _ := b.Load(b.Head())
	if len(head.Parents) != 2 || !((head.Parents[0] == m.ID && head.Parents[1] == v2.ID) || (head.Parents[0] == v2.ID && head.Parents[1] == m.ID)) {
		t.Fatalf("merge parents %v, want %s and %s", head.Parents, m.ID, v2.ID)
	}
	if _, err := os.Stat(filepath.Join(b.Root, "Samples", "older-idea.wav")); err != nil {
		t.Error("the change made on the older version is gone")
	}
	if setTracks(t, b)["Audios"].Elem == nil {
		t.Error("the latest version's changes are missing")
	}
}
