package desktop

import (
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

// The reminder to back up waits until the team is settled here: three days
// after joining, or three projects. Someone trying DAWGit out isn't told
// what could go wrong.
func TestBackupReminderWaitsUntilSettled(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("band")
	defer fake.Close()
	t.Cleanup(waitTidy)
	a := NewApp()
	team, err := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k", SecretKey: "s"}, "Band")
	if err != nil {
		t.Fatal(err)
	}
	updateTeam(team.ID, func(_ *teams.Store, tm *teams.Team) error {
		tm.MemberID, tm.MemberName = teams.NewID(16), "Yi"
		return nil
	})
	if a.BackupReminder(team.ID) {
		t.Error("reminded on a new team")
	}
	// Three projects: settled.
	for i := 0; i < 3; i++ {
		root := newSong(t)
		if _, err := a.AddProjectToTeam(team.ID, root); err != nil {
			t.Fatal(err)
		}
	}
	if !a.BackupReminder(team.ID) {
		t.Error("not reminded with three projects")
	}

	// Or three days.
	store, _ := teams.Load()
	tm := *store.Find(team.ID)
	store.Projects = map[string]string{}
	if settled(store, tm, tm.Added.Add(2*24*time.Hour)) {
		t.Error("settled after two days")
	}
	if !settled(store, tm, tm.Added.Add(3*24*time.Hour)) {
		t.Error("not settled after three days")
	}
	tm.Added = time.Time{}
	if !settled(store, tm, time.Now()) {
		t.Error("a team from before it was noted isn't settled")
	}
}
