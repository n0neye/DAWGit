package desktop

import (
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// With the team's storage down, LocalOverview still lists the projects here
// at once (the app starts on it), and Overview says the team can't be reached.
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
	waitTidy()
	fake.Close()

	start := time.Now()
	ov, err := a.LocalOverview()
	if err != nil || len(ov.Projects) != 1 || ov.Projects[0].Root != root || ov.TeamChecked || ov.TeamError != "" {
		t.Fatalf("local: %+v %v", ov, err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("LocalOverview took %v", d)
	}
	ov, err = a.Overview()
	if err != nil || len(ov.Projects) != 1 || !ov.TeamChecked || ov.TeamError == "" {
		t.Fatalf("with the team: %+v %v", ov, err)
	}
}
