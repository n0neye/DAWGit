package main

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

func newSong(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Song Project")
	os.MkdirAll(root, 0o755)
	data, err := os.ReadFile(filepath.Join("..", "SampleProjects", "SampleAbletonProject Project", "SampleAbletonProject_v2.als"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "Song.als"), data, 0o644)
	return root
}

func TestDisconnectReconnectAndMove(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("one", "two")
	defer fake.Close()
	a := NewApp()
	one, _ := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "one", AccessKey: "k", SecretKey: "s"}, "One")
	root := newSong(t)
	if _, err := a.AddProjectToTeam(one.ID, root); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(root, "first", true, nil, true); err != nil {
		t.Fatal(err)
	}

	// Disconnect, keeping the project: it is under Local with its versions.
	code, _ := a.TeamConnectionCode(one.ID)
	if err := a.RemoveTeam(one.ID, true); err != nil {
		t.Fatal(err)
	}
	store, _ := teams.Load()
	if len(store.Local) != 1 || store.Local[0] != root {
		t.Fatalf("local after disconnect: %v", store.Local)
	}
	r, _ := project.Open(root)
	if r.Config.Remote != nil || r.Head() == "" {
		t.Fatalf("detached project: remote %v head %q", r.Config.Remote, r.Head())
	}

	// Join again: the project is found and can be reconnected, no new version.
	again, err := a.ConnectTeam(code, "")
	if err != nil {
		t.Fatal(err)
	}
	found, err := a.TeamProjectsHere(again.ID)
	if err != nil || len(found) != 1 || found[0].Root != root {
		t.Fatalf("found: %v %+v", err, found)
	}
	if err := a.ReconnectProjects(again.ID, []string{root}); err != nil {
		t.Fatal(err)
	}
	store, _ = teams.Load()
	if len(store.Local) != 0 || store.ProjectRoot(again.ID, r.Config.ProjectID) != root {
		t.Fatalf("not reconnected: local %v projects %v", store.Local, store.Projects)
	}
	if res, err := a.Save(root, "First version", true, nil, true); err != nil || res.Action == "published" {
		t.Fatalf("reconnecting made a version: %+v %v", res, err)
	}

	// Move it to another team: shared there, still in the first one.
	two, _ := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "two", AccessKey: "k", SecretKey: "s"}, "Two")
	if _, err := a.MoveProjectToTeam(root, two.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(root, "First version", true, nil, true); err != nil {
		t.Fatal(err)
	}
	store, _ = teams.Load()
	if store.ProjectRoot(two.ID, r.Config.ProjectID) != root || store.ProjectRoot(again.ID, r.Config.ProjectID) != "" {
		t.Fatalf("after move: %v", store.Projects)
	}
	for _, id := range []string{again.ID, two.ID} {
		b, _ := remote.Open(store.Find(id).Remote)
		if ps, _ := b.Projects(); len(ps) != 1 {
			t.Errorf("team %s projects: %+v", store.Find(id).Name, ps)
		}
	}
	// Move to Local only.
	if err := a.MoveProjectToLocal(root); err != nil {
		t.Fatal(err)
	}
	store, _ = teams.Load()
	if len(store.Local) != 1 || store.ProjectRoot(two.ID, r.Config.ProjectID) != "" {
		t.Fatalf("after move to local: local %v projects %v", store.Local, store.Projects)
	}
}
