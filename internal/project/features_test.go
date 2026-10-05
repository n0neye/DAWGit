package project

import (
	"errors"
	"path/filepath"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// A team that turned on a feature this build doesn't know can't be joined:
// the error says to update or switch to Nightly. (Working with a team
// already joined is checked in remote.CheckFeatures, through teams.Open.)
func TestJoinNeedsTeamFeatures(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	if _, err := Connect(code, ""); err != nil {
		t.Fatal("no features:", err)
	}
	fake.Put("team", "dawgit/team.json", []byte(`{"name":"Band","features":["from-the-future"]}`))
	var tf *remote.ErrTeamFeatures
	if _, err := Connect(code, ""); !errors.As(err, &tf) || tf.Missing[0] != "from-the-future" {
		t.Fatalf("join: %v", err)
	}
}

// Sharing moves the team's branch, and the move is recorded.
func TestBranchLogRecordsShares(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "notes.txt", "one")
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "notes.txt", "two")
	if _, _, err := a.Save("second", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	moves, _, err := a.BranchLog("main")
	if err != nil || len(moves) != 2 || moves[0].From != "" || moves[1].From != moves[0].To || moves[1].To != a.Head() {
		t.Fatalf("log: %+v %v", moves, err)
	}
}

// Committing some changes while the team is ahead: the team's versions come
// in first, the picked changes are committed, and the others stay on disk,
// uncommitted.
func TestPartialCommitWhileTeamAhead(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt", "c.txt", "team.txt"} {
		write(t, a.Root, f, "first")
	}
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", a.Config.Name, filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	write(t, b.Root, "team.txt", "from the team")
	if _, _, err := b.Save("team edit", Strategy("fail")); err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		write(t, a.Root, f, "mine")
	}
	a.Only = []string{"a.txt"}
	if _, _, err := a.Save("only a", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	a.Only = nil
	got := files(t, a.Root)
	if got["a.txt"] != "mine" || got["b.txt"] != "mine" || got["c.txt"] != "mine" || got["team.txt"] != "from the team" {
		t.Fatalf("files after the commit: a %q, b %q, c %q, team %q", got["a.txt"], got["b.txt"], got["c.txt"], got["team.txt"])
	}
	head, _ := a.Load(a.Head())
	committed := map[string]bool{}
	first, _ := a.Load(head.Parents[0])
	for _, f := range head.Files {
		committed[f.Path] = true
		for _, g := range first.Files {
			if g.Path == f.Path && g.Hash != f.Hash && f.Path != "a.txt" && f.Path != "team.txt" {
				t.Errorf("%s was committed too", f.Path)
			}
		}
	}
	changes, _ := a.Status()
	left := map[string]bool{}
	for _, c := range changes {
		left[c.Path] = true
	}
	if !left["b.txt"] || !left["c.txt"] || left["a.txt"] {
		t.Errorf("uncommitted after: %v", changes)
	}
}
