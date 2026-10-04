package backup

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func TestRestore(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	team, _ := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	song, demo := strings.Repeat("1", 32), strings.Repeat("2", 32)
	kick := sum("kick")
	team.PutObject(kick, bytes.NewReader([]byte("kick")))
	v1, v2, d1 := sum(`{"v":1}`), sum(`{"v":2}`), sum(`{"d":1}`)
	team.PutProject(remote.Project{ID: song, Name: "Song"})
	team.PutSnapshot(song, v1, []byte(`{"v":1}`))
	team.UpdateBranch(song, "main", "", v1)
	team.PutProject(remote.Project{ID: demo, Name: "Demo"})
	team.PutSnapshot(demo, d1, []byte(`{"d":1}`))
	team.UpdateBranch(demo, "main", "", d1)

	dst := Folder(t.TempDir())
	if err := Claim(dst, "t1", "Band"); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(team, dst, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	team.PutSnapshot(song, v2, []byte(`{"v":2}`))
	team.UpdateBranch(song, "main", v1, v2)
	team.UpdateBranch(song, "idea", "", v2)
	if _, err := Run(team, dst, nil); err != nil {
		t.Fatal(err)
	}
	runs, _ := Runs(dst)
	if len(runs) != 2 {
		t.Fatalf("runs %v", runs)
	}

	// Nothing lost: nothing to do.
	p, err := MakePlan(dst, team, "")
	if err != nil || !p.Empty() || p.Team != "Band" || len(p.Runs) != 2 {
		t.Fatalf("plan with nothing lost: %+v %v", p, err)
	}

	// Demo deleted, a file lost, Song's idea branch deleted, Song's main moved on.
	team.DeleteProject(demo)
	fake.Delete("band", "team/objects/"+kick[:2]+"/"+kick[2:])
	team.UpdateBranch(song, "idea", v2, "")
	team.UpdateBranch(song, "main", v2, v1)
	p, err = MakePlan(dst, team, "")
	if err != nil || len(p.Projects) != 1 || p.Projects[0].Name != "Demo" || p.Projects[0].Versions != 1 || p.Branches != 1 {
		t.Fatalf("plan: %+v %v", p, err)
	}
	if _, err := Restore(dst, team, p, nil); err != nil {
		t.Fatal(err)
	}
	ps, _ := team.Projects()
	if len(ps) != 2 {
		t.Errorf("projects back: %+v", ps)
	}
	if h, _ := team.BranchHead(demo, "main"); h != d1 {
		t.Errorf("demo main %q", h)
	}
	if h, _ := team.BranchHead(song, "idea"); h != v2 {
		t.Errorf("song idea %q", h)
	}
	if h, _ := team.BranchHead(song, "main"); h != v1 {
		t.Errorf("a branch the team has is left as it is: %q", h)
	}
	if r, err := team.GetObject(kick); err != nil {
		t.Error("lost file back:", err)
	} else {
		r.Close()
	}
	if p, _ := MakePlan(dst, team, ""); !p.Empty() {
		t.Errorf("again: %+v", p)
	}

	// As of the first run: Song's idea branch (made later) stays out.
	team.DeleteProject(song)
	p, err = MakePlan(dst, team, runs[1])
	if err != nil || len(p.Projects) != 1 || p.Projects[0].Name != "Song" {
		t.Fatalf("plan at the first run: %+v %v", p, err)
	}
	if _, err := Restore(dst, team, p, nil); err != nil {
		t.Fatal(err)
	}
	if h, _ := team.BranchHead(song, "main"); h != v1 {
		t.Errorf("main as at the first run: %q", h)
	}
	if h, _ := team.BranchHead(song, "idea"); h != "" {
		t.Errorf("idea came after the first run: %q", h)
	}
	if _, err := MakePlan(dst, team, "nope"); err != ErrNoRun {
		t.Error("unknown run:", err)
	}
	if _, err := MakePlan(Folder(t.TempDir()), team, ""); err != ErrNotBackup {
		t.Error("not a backup:", err)
	}
}
