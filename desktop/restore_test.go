package desktop

import (
	"errors"
	"testing"

	"dawgit/internal/backup"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

// Restoring from a backup in a bucket this computer doesn't back up to (a
// teammate's, say): its address and keys are given for the restore.
func TestRestoreFromAnotherBucket(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("band", "vault")
	defer fake.Close()
	t.Cleanup(waitTidy)
	a := NewApp()
	team, err := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k", SecretKey: "s"}, "Band")
	if err != nil {
		t.Fatal(err)
	}
	root := newSong(t)
	if _, err := a.AddProjectToTeam(team.ID, root); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Save(root, "first", true, nil, true, nil); err != nil {
		t.Fatal(err)
	}
	waitTidy()

	// A teammate backs the team up into another bucket.
	vault := remote.Storage{Endpoint: fake.URL, Bucket: "vault", Folder: "band-backup", AccessKey: "k", SecretKey: "s"}
	cfg, _ := vault.Config()
	d, err := backup.Bucket(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := teams.Load()
	tm := store.Find(team.ID)
	s3, err := backup.Storage(*tm)
	if err != nil {
		t.Fatal(err)
	}
	if err := backup.Claim(d, tm.ID, tm.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.Run(s3, d, nil); err != nil {
		t.Fatal(err)
	}
	// The project is deleted from the team.
	ps, _ := s3.Projects()
	if err := s3.DeleteProject(ps[0].ID); err != nil {
		t.Fatal(err)
	}

	if tm.Backup != nil {
		t.Fatal("this computer backs up")
	}
	if _, err := a.RestorePlan(team.ID, RestoreSource{}, ""); err == nil {
		t.Fatal("no backup here, none given: should say so")
	}
	own := remote.Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k", SecretKey: "s"}
	if _, err := a.RestorePlan(team.ID, RestoreSource{Storage: &own}, ""); !errors.Is(err, ErrOwnStorage) {
		t.Fatalf("from the team's own storage: %v", err)
	}
	from := RestoreSource{Storage: &vault}
	plan, err := a.RestorePlan(team.ID, from, "")
	if err != nil || len(plan.Projects) != 1 || plan.Projects[0].Name != "Song" || plan.Files == 0 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if n, err := a.Restore(team.ID, from, ""); err != nil || n == 0 {
		t.Fatalf("restore: %d %v", n, err)
	}
	if ps, _ := s3.Projects(); len(ps) != 1 || ps[0].Name != "Song" {
		t.Fatalf("after: %+v", ps)
	}
	if store, _ := teams.Load(); store.Find(team.ID).Backup != nil {
		t.Error("restoring set up a backup here")
	}
}
