package main

import (
	"strings"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

func TestStorageTeamSetup(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("band")
	defer fake.Close()
	a := NewApp()

	s := remote.Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k1", SecretKey: "s1"}
	if _, err := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "nope", AccessKey: "k", SecretKey: "s"}, "X"); err == nil {
		t.Fatal("a missing bucket made a team")
	}
	team, err := a.CreateStorageTeam(s, "Night Shift")
	if err != nil || team.Name != "Night Shift" || !team.IsStorage {
		t.Fatalf("create: %+v %v", team, err)
	}
	code, err := a.TeamConnectionCode(team.ID)
	if err != nil || !remote.IsConnectionCode(code) {
		t.Fatalf("code: %q %v", code, err)
	}
	// Someone creating the team again keeps the name already there.
	if again, err := a.CreateStorageTeam(s, "Other"); err != nil || again.ID != team.ID || again.Name != "Night Shift" {
		t.Fatalf("again: %+v %v", again, err)
	}

	c, err := a.TeamConnectionSettings(team.ID)
	if err != nil || !c.Storage || c.Settings.Bucket != "band" || c.Settings.SecretKey != "s1" {
		t.Fatalf("settings: %+v %v", c, err)
	}
	c.Settings.AccessKey, c.Settings.SecretKey = "k2", "s2"
	if _, err := a.UpdateTeamConnection(team.ID, c); err != nil {
		t.Fatal(err)
	}
	store, _ := teams.Load()
	if got := store.Find(team.ID).Remote; got.AccessKey != "k2" || got.SecretKey != "s2" {
		t.Fatalf("keys not updated: %+v", got)
	}
	c.Settings.Bucket = "gone"
	if _, err := a.UpdateTeamConnection(team.ID, c); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("bad bucket accepted: %v", err)
	}
}
