package backup

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

func TestBucketBackup(t *testing.T) {
	fake := s3test.New("band", "vault")
	defer fake.Close()
	team, err := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.Repeat("1", 32)
	h := sum("kick")
	team.PutObject(h, bytes.NewReader([]byte("kick")))
	team.PutProject(remote.Project{ID: pid, Name: "Song"})
	v1, v2 := sum(`{"version":1}`), sum(`{"version":2}`)
	team.PutSnapshot(pid, v1, []byte(`{"version":1}`))
	team.UpdateBranch(pid, "main", "", v1)

	cfg := remote.Config{URL: "s3+" + fake.URL + "/vault/band-backup", AccessKey: "k", SecretKey: "s", Region: "auto"}
	if err := remote.CheckBackup(cfg); err != nil {
		t.Fatal("check:", err)
	}
	d, err := Bucket(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := Claimed(d, "t1"); err != ErrMissing {
		t.Fatal("not claimed yet:", err)
	}
	if err := Claim(d, "t1", "Band"); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(team, d, nil)
	if err != nil || rep.Copied != rep.Keys || rep.Copied == 0 {
		t.Fatalf("first run %+v %v", rep, err)
	}
	vault, _ := remote.NewS3(fake.URL, "vault", "band-backup", "auto", "k", "s")
	if got, err := d.Read("objects/" + h[:2] + "/" + h[2:]); err != nil || string(got) != "kick" {
		t.Fatalf("object %q %v", got, err)
	}

	// Nothing new: nothing copied, the branch included.
	time.Sleep(1100 * time.Millisecond)
	if rep, err := Run(team, d, nil); err != nil || rep.Copied != 0 {
		t.Fatalf("second run %+v %v", rep, err)
	}
	// The team moves on and deletes: the branch is copied again, nothing goes.
	time.Sleep(1100 * time.Millisecond)
	team.PutSnapshot(pid, v2, []byte(`{"version":2}`))
	team.UpdateBranch(pid, "main", v1, v2)
	fake.Delete("band", "team/objects/"+h[:2]+"/"+h[2:])
	if rep, err := Run(team, d, nil); err != nil || rep.Copied != 2 {
		t.Fatalf("third run %+v %v", rep, err)
	}
	if b, _ := d.Read("projects/" + pid + "/branches/main"); !strings.Contains(string(b), v2) {
		t.Errorf("branch %q", b)
	}
	if _, err := d.Read("objects/" + h[:2] + "/" + h[2:]); err != nil {
		t.Error("a backup never deletes:", err)
	}
	runs, _ := vault.List("runs/")
	if len(runs) != 3 {
		t.Errorf("runs %v", runs)
	}
	if err := Claim(d, "t2", "Other"); err != ErrOtherTeam {
		t.Error("another team's:", err)
	}

	// Not into the team's own storage.
	tm := teams.Team{Remote: remote.Config{URL: "s3+" + fake.URL + "/band/team"}}
	for url, want := range map[string]bool{
		"/band/team": true, "/band/team/backup": true, "/band/teamwork": false, "/vault/team": false,
	} {
		if Overlaps(remote.Config{URL: "s3+" + fake.URL + url}, tm) != want {
			t.Errorf("overlaps %s: want %v", url, want)
		}
	}
	full := remote.Config{URL: "s3+" + fake.URL + "/band/other", AccessKey: "k", SecretKey: "s"}
	other, _ := Bucket(full)
	other.Write("song.als", []byte("x"))
	if err := Claim(other, "t1", "Band"); err != ErrNotEmpty {
		t.Error("a bucket folder with things in it:", err)
	}
}
