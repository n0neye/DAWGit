package project

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/manifest"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/store"
)

func unfixed(t *testing.T, rep *VerifyReport) []Problem {
	t.Helper()
	var out []Problem
	for _, p := range rep.Problems {
		if !p.Fixed {
			out = append(out, p)
		}
	}
	return out
}

// A damaged stored file is found, and comes back from the project folder;
// one with no copy anywhere is reported.
func TestVerifyLocalProject(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	os.WriteFile(filepath.Join(root, "kick.wav"), []byte("kick"), 0o644)
	os.WriteFile(filepath.Join(root, "old.wav"), []byte("old take"), 0o644)
	mustSnapshot(t, r, "first")
	rep, err := r.Verify(false)
	if err != nil || len(rep.Problems) != 0 || rep.Versions != 1 || rep.Files == 0 || rep.Folders == 0 {
		t.Fatalf("clean project: %+v %v", rep, err)
	}

	kick, _, _ := store.HashFile(filepath.Join(root, "kick.wav"))
	old, _, _ := store.HashFile(filepath.Join(root, "old.wav"))
	os.WriteFile(r.Store.Path(kick), []byte("kicx"), 0o644) // a bad sector
	os.Remove(r.Store.Path(old))
	os.Remove(filepath.Join(root, "old.wav")) // gone from the folder too
	mustSnapshot(t, r, "without old.wav")

	rep, _ = r.Verify(false)
	if len(rep.Problems) != 2 {
		t.Fatalf("problems: %+v", rep.Problems)
	}
	rep, _ = r.Verify(true)
	left := unfixed(t, rep)
	if len(left) != 1 || left[0].Path != "old.wav" || left[0].Kind != "file" {
		t.Fatalf("after repair: %+v", rep.Problems)
	}
	if got, _, _ := store.HashFile(r.Store.Path(kick)); got != kick {
		t.Fatal("kick.wav not repaired")
	}
	t.Log(rep.Summary())
}

// In a team, a lost folder list and a damaged file come back from the
// team's storage.
func TestVerifyTeamProject(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	r, _ := Init(newProject(t), "yi")
	if err := r.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(r.Root, "Stems"), 0o755)
	os.WriteFile(filepath.Join(r.Root, "Stems", "bass.wav"), []byte("bass"), 0o644)
	if _, _, err := r.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	head, _ := r.Header(r.Head())
	var stems string
	for _, e := range mustTree(t, r, head.Tree) {
		if e.Name == "Stems" {
			stems = e.Hash
		}
	}
	os.Remove(r.treePath(stems))
	set, _, _ := store.HashFile(filepath.Join(r.Root, "Song.als")) // sets stay here
	os.WriteFile(r.Store.Path(set), []byte("damaged"), 0o644)
	os.WriteFile(filepath.Join(r.Root, "Song.als"), []byte("edited since"), 0o644) // no other copy here

	rep, err := r.Verify(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 2 || len(unfixed(t, rep)) != 0 || !rep.TeamChecked {
		t.Fatalf("problems: %+v", rep.Problems)
	}
	if rep, _ := r.Verify(false); len(rep.Problems) != 0 {
		t.Fatalf("after repair: %+v", rep.Problems)
	}
}

// A teammate's version whose files were never downloaded here (replaced
// since) is fine: the team has them.
func TestVerifyTeammatesUndownloadedFiles(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "notes.txt", "one")
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"two", "three"} {
		write(t, b.Root, "notes.txt", s)
		if _, _, err := b.Save(s, Strategy("fail")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	rep, err := a.Verify(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Problems) != 0 || !rep.TeamChecked {
		t.Fatalf("problems: %+v", rep.Problems)
	}
}

func mustTree(t *testing.T, r *Repo, h string) []manifest.TreeEntry {
	t.Helper()
	es, err := r.readTreeFromDisk(h)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// A shared project's files and folder lists all count as used by storage
// cleanup: nothing of it waits to be deleted.
func TestStorageCleanupKeepsSharedProject(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	cfg := remote.Config{URL: "s3+" + fake.URL + "/team/dawgit", AccessKey: "key", SecretKey: "secret"}
	r, _ := Init(newProject(t), "yi")
	if err := r.SetRemote(remote.EncodeConnectionCode(cfg), ""); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(r.Root, "Stems"), 0o755)
	os.WriteFile(filepath.Join(r.Root, "Stems", "bass.wav"), []byte("bass"), 0o644)
	if _, _, err := r.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(r.Root, "Stems", "bass.wav"), []byte("bass 2"), 0o644)
	if _, _, err := r.Save("second", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _ := remote.Open(cfg)
	rep, err := b.(*remote.S3Backend).CollectGarbage(true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Versions != 2 || rep.Waiting != 0 || rep.Deleted != 0 || rep.Used != rep.Stored {
		t.Fatalf("cleanup of a shared project: %+v", rep)
	}
}
