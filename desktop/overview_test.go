package desktop

import (
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

// With the team's storage down, LocalOverview still lists the projects at
// once (the app starts on it): those here, and the team's others as it last
// listed them. Overview says the team can't be reached.
func TestOverviewWithTheTeamDown(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	defer func(w time.Duration) { remote.RetryWait = w }(remote.RetryWait)
	remote.RetryWait = time.Millisecond
	fake := s3test.New("one")
	t.Cleanup(waitTidy)
	a := NewApp()
	team, err := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "one", AccessKey: "k", SecretKey: "s"}, "One")
	if err != nil {
		t.Fatal(err)
	}
	root := newSong(t)
	if _, err := a.AddProjectToTeam(team.ID, root); err != nil {
		t.Fatal(err)
	}
	store, _ := teams.Load()
	c, err := store.Find(team.ID).Open()
	if err != nil {
		t.Fatal(err)
	}
	c.PutProject(remote.Project{ID: teams.NewID(16), Name: "Other"}) // not downloaded here
	if ov, err := a.Overview(); err != nil || len(ov.Projects) != 2 {
		t.Fatalf("up: %+v %v", ov, err)
	}
	waitTidy()
	fake.Close()
	remotes := func(ov *Overview) (names []string) {
		for _, p := range ov.Projects {
			if p.Status == "remote" {
				names = append(names, p.Name)
			}
		}
		return names
	}

	start := time.Now()
	ov, err := a.LocalOverview()
	if err != nil || len(ov.Projects) != 2 || ov.TeamChecked || ov.TeamError != "" {
		t.Fatalf("local: %+v %v", ov, err)
	}
	if r := remotes(ov); len(r) != 1 || r[0] != "Other" {
		t.Fatalf("local, the team's: %v", r)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("LocalOverview took %v", d)
	}
	ov, err = a.Overview()
	if err != nil || !ov.TeamChecked || ov.TeamError == "" || len(remotes(ov)) != 1 {
		t.Fatalf("with the team: %+v %v", ov, err)
	}
}
