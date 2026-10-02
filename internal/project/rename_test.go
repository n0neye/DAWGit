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
