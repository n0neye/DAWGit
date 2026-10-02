package project

import (
	"path/filepath"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// A project renamed by one person keeps its name when the others save.
func TestRenameStaysForTheTeam(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "Notes/lyrics.txt", lyrics)
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Rename("  "); err == nil {
		t.Error("an empty name is refused")
	}
	if err := a.Rename("Night Drive"); err != nil {
		t.Fatal(err)
	}
	if again, _ := Open(a.Root); again.Config.Name != "Night Drive" {
		t.Errorf("local name: %q", again.Config.Name)
	}
	write(t, b.Root, "Notes/lyrics.txt", lyrics+"line 9\n")
	if _, _, err := b.Save("more", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	c, _ := a.Client()
	ps, err := c.Projects()
	if err != nil || len(ps) != 1 || ps[0].Name != "Night Drive" {
		t.Fatalf("team's name after another save: %+v %v", ps, err)
	}
}

// A project with versions here joins a team: Share uploads them without
// committing the files.
func TestShareVersionsLater(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	write(t, a.Root, "Notes/lyrics.txt", lyrics)
	first := mustSnapshot(t, a, "here only")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "Notes/lyrics.txt", lyrics+"not committed\n")
	res, err := a.Share(Strategy("fail"))
	if err != nil || res.Action != "published" || a.Head() != first.ID {
		t.Fatalf("share: %+v %v (head %s)", res, err, a.Head())
	}
	c, _ := a.Client()
	if bs, _ := c.Branches(a.Config.ProjectID); bs["main"] != first.ID {
		t.Fatalf("team's main: %v", bs)
	}
	if changes, _ := a.Status(); len(changes) != 1 {
		t.Fatalf("the uncommitted change stays: %+v", changes)
	}
}
