package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/teams"
)

// actAs makes the team's member on this computer id (A and B share one).
func actAs(t *testing.T, r *Repo, id string) {
	t.Helper()
	store, _ := teams.Load()
	tm := store.FindByURL(r.Config.Remote.URL)
	tm.MemberID, tm.MemberName = id, id[:4]
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
}

// takeBackTeam: A shared v1 (a, b), B has it, then A shared a wrong
// version (a changed, c added) that B hasn't taken in.
func takeBackTeam(t *testing.T) (a, b *Repo, code, wrong, yi, alex string) {
	t.Helper()
	fake := s3test.New("team")
	t.Cleanup(fake.Close)
	code = remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	yi, alex = teams.NewID(16), teams.NewID(16)
	a, _ = Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	actAs(t, a, yi)
	write(t, a.Root, "a.txt", "a1")
	write(t, a.Root, "b.txt", "b1")
	if _, _, err := a.Save("v1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	actAs(t, a, alex)
	b, _, err := Clone(code, "", a.Config.Name, filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	actAs(t, a, yi)
	write(t, a.Root, "a.txt", "a wrong")
	write(t, a.Root, "c.txt", "should not be here")
	m, _, err := a.Save("wrong", Strategy("fail"))
	if err != nil {
		t.Fatal(err)
	}
	return a, b, code, m.ID, yi, alex
}

// Taking back the latest version: gone from the team, its changes back here
// uncommitted.
func TestTakeBack(t *testing.T) {
	a, b, code, wrong, _, alex := takeBackTeam(t)
	v1, _ := a.Resolve("HEAD~1")
	p, err := a.PlanTakeBack(wrong)
	if err != nil || !p.OK || !p.Shared || !p.FeatureOff {
		t.Fatalf("plan: %+v %v", p, err)
	}
	if _, err := a.TakeBack(wrong, false); !errors.Is(err, ErrTakeBackOff) {
		t.Fatalf("feature off: %v", err)
	}
	if _, err := a.TakeBack(wrong, true); err != nil {
		t.Fatal(err)
	}
	if a.Head() != v1 {
		t.Fatalf("head %s, want v1", short(a.Head()))
	}
	got := files(t, a.Root)
	if got["a.txt"] != "a wrong" || got["c.txt"] != "should not be here" {
		t.Fatalf("files changed: %v", got)
	}
	if ch, _ := a.Status(); len(ch) != 2 {
		t.Fatalf("uncommitted: %+v", ch)
	}
	c, _ := a.Client()
	if heads, _ := c.Branches(a.Config.ProjectID); heads["main"] != v1 {
		t.Fatalf("team head %s", short(heads["main"]))
	}
	if info, _ := c.Info(); len(info.Features) != 1 || info.Features[0] != FeatureTakeBack {
		t.Fatalf("features %v", info.Features)
	}
	// B never sees it.
	actAs(t, b, alex)
	if res, err := b.Update(Strategy("fail")); err != nil || res.Action != "up-to-date" {
		t.Fatalf("B update: %+v %v", res, err)
	}
	if c := cloneFiles(t, code, a.Config.Name); c["a.txt"] != "a1" || c["c.txt"] != "" {
		t.Fatalf("a clone gets %v", fileNames(c))
	}
}

// A version a teammate took in can't be taken back; nor can someone
// else's, or one before the latest.
func TestTakeBackRefused(t *testing.T) {
	a, b, _, wrong, yi, alex := takeBackTeam(t)
	actAs(t, b, alex)
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if p, _ := b.PlanTakeBack(wrong); p.Why != "not-yours" {
		t.Errorf("B: %+v", p)
	}
	actAs(t, a, yi)
	p, err := a.PlanTakeBack(wrong)
	if err != nil || p.OK || p.Why != "has-it" || len(p.HaveIt) != 1 || p.HaveIt[0] != alex {
		t.Fatalf("plan: %+v %v", p, err)
	}
	if _, err := a.TakeBack(wrong, true); err == nil {
		t.Fatal("taken back")
	}
	v1, _ := a.Resolve("HEAD~1")
	if p, _ := a.PlanTakeBack(v1); p.Why != "not-latest" {
		t.Errorf("v1: %+v", p)
	}
}

// A teammate who took the version in at the same moment takes it out again
// when sharing, keeping their own work; so does a copy from before the
// records (no version seen kept).
func TestTakenBackIsDropped(t *testing.T) {
	for _, tracked := range []bool{true, false} {
		a, b, code, wrong, yi, alex := takeBackTeam(t)
		actAs(t, b, alex)
		if _, err := b.Update(Strategy("fail")); err != nil {
			t.Fatal(err)
		}
		if !tracked {
			os.Remove(filepath.Join(b.Dir, seenFile))
		}
		// B commits on top of it (not shared) and has more uncommitted.
		write(t, b.Root, "b.txt", "b by alex")
		if _, err := b.Snapshot("alex's"); err != nil {
			t.Fatal(err)
		}
		write(t, b.Root, "notes.txt", "uncommitted")
		// A takes it back regardless (B's record not seen in time).
		actAs(t, a, yi)
		v1, _ := a.Resolve("HEAD~1")
		c, _ := a.Client()
		if err := c.UpdateBranch(a.Config.ProjectID, "main", wrong, v1); err != nil {
			t.Fatal(err)
		}
		actAs(t, b, alex)
		if in, err := b.Incoming(); err != nil || !in {
			t.Fatalf("tracked %v: incoming %v %v", tracked, in, err)
		}
		res, err := b.Share(Strategy("fail"))
		if err != nil || len(res.TakenBack) != 1 || res.TakenBack[0].ID != wrong || res.Action != "published" {
			t.Fatalf("tracked %v: share %+v %v", tracked, res, err)
		}
		got := files(t, b.Root)
		if got["a.txt"] != "a1" || got["c.txt"] != "" || got["b.txt"] != "b by alex" || got["notes.txt"] != "uncommitted" {
			t.Fatalf("tracked %v: B's files %v", tracked, got)
		}
		team := cloneFiles(t, code, a.Config.Name)
		if team["a.txt"] != "a1" || team["c.txt"] != "" || team["b.txt"] != "b by alex" || team["notes.txt"] != "" {
			t.Fatalf("tracked %v: the team has %v", tracked, fileNames(team))
		}
		if in, _ := b.ancestors(b.Head()); in[wrong] {
			t.Fatalf("tracked %v: still in B's history", tracked)
		}
	}
}

// Without a team, the latest version just goes.
func TestTakeBackLocal(t *testing.T) {
	r, _ := Init(newProject(t), "yi")
	write(t, r.Root, "a.txt", "1")
	first, _ := r.Snapshot("one")
	write(t, r.Root, "a.txt", "2")
	second, _ := r.Snapshot("two")
	if _, err := r.TakeBack(second.ID, false); err != nil {
		t.Fatal(err)
	}
	if r.Head() != first.ID {
		t.Fatal("head")
	}
	if ch, _ := r.Status(); len(ch) != 1 || ch[0].Path != "a.txt" {
		t.Fatalf("uncommitted: %+v", ch)
	}
}
