package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/store"
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
	os.MkdirAll(filepath.Join(root, "Samples"), 0o755)
	os.WriteFile(filepath.Join(root, "Samples", "kick.wav"), []byte("RIFF-kick"), 0o644)
	return root
}

func TestDisconnectAndReconnect(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("one", "two")
	defer fake.Close()
	t.Cleanup(waitTidy)
	a := NewApp()
	one, _ := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "one", AccessKey: "k", SecretKey: "s"}, "One")
	root := newSong(t)
	if _, err := a.AddProjectToTeam(one.ID, root); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(root, "first", true, nil, true, nil); err != nil {
		t.Fatal(err)
	}

	// Disconnect, keeping the project: it keeps its versions, set aside.
	code, _ := a.TeamConnectionCode(one.ID)
	waitTidy()
	kick, _, _ := store.HashFile(filepath.Join(root, "Samples", "kick.wav"))
	if r, _ := project.Open(root); r.Store.Has(kick) {
		t.Fatal("the sample is still copied in .dawgit after the team has it")
	}
	if size, err := a.HistoryDownloadSize("", one.ID); err != nil || size != 0 {
		t.Fatalf("one version, all of it here: %d %v", size, err)
	}
	if err := a.RemoveTeam(one.ID, true, false); err != nil {
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
	if res, err := a.Save(root, "First version", true, nil, true, nil); err != nil || res.Action == "published" {
		t.Fatalf("reconnecting made a version: %+v %v", res, err)
	}
}
