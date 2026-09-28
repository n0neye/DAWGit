package project

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchWorkflow(t *testing.T) {
	a, b := team(t)

	// Yi experiments on a personal branch; main is untouched.
	if err := a.CreateBranch("yi-ideas"); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateBranch("yi-ideas"); err == nil {
		t.Error("creating an existing branch should fail")
	}
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, res, err := a.Save("group audio", "fail"); err != nil || res.Action != "published" {
		t.Fatalf("save on branch: %v %+v", err, res)
	}
	if up, err := b.Update("fail"); err != nil || up.Action != "up-to-date" {
		t.Fatalf("main should not have moved: %v %+v", err, up)
	}

	// Alex keeps working on main.
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	if _, _, err := b.Save("drums", "fail"); err != nil {
		t.Fatal(err)
	}

	branches, err := b.Branches()
	if err != nil || len(branches) != 2 || branches[0].Name != "main" || !branches[0].Current ||
		branches[1].Latest == nil || branches[1].Latest.Message != "group audio" {
		t.Fatalf("branches: %v %+v", err, branches)
	}

	// Alex previews Yi's branch: what comes in and what will conflict.
	p, err := b.PreviewMerge("yi-ideas")
	if err != nil {
		t.Fatal(err)
	}
	if p.Action != "merge" || len(p.Versions) != 1 || p.Versions[0].Message != "group audio" {
		t.Fatalf("preview: %+v", p)
	}
	if len(p.Changes) != 1 || p.Changes[0].SetDiff == nil ||
		!strings.Contains(p.Changes[0].SetDiff.Render(), `GroupTrack "Audios"`) {
		t.Fatalf("preview changes: %+v", p.Changes)
	}
	if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], "Bounce + Reverb") {
		t.Fatalf("preview conflicts: %v", p.Conflicts)
	}
	assertClean(t, b) // preview changes nothing

	res, err := b.MergeBranch("yi-ideas", "both")
	if err != nil || res.Action != "merged" {
		t.Fatalf("merge branch: %v %+v", err, res)
	}
	if setTracks(t, b)["Audios"].Elem == nil {
		t.Error("merge did not bring the group")
	}
	m, _ := b.Load(b.Head())
	if m.Message != "Merge branch yi-ideas" {
		t.Errorf("merge message = %q", m.Message)
	}

	// Yi goes back to main and gets everything.
	sw, err := a.SwitchBranch("main", false)
	if err != nil || sw.To != b.Head() || a.BranchName() != "main" {
		t.Fatalf("switch: %v %+v", err, sw)
	}
	if setTracks(t, a)["Drum"].Elem == nil {
		t.Error("switch did not bring main's drum track")
	}
	assertClean(t, a)
}

func TestSwitchRefusesUnsharedVersions(t *testing.T) {
	a, _ := team(t)
	a.CreateBranch("draft")
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, err := a.Snapshot("local only"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SwitchBranch("main", false); !errors.Is(err, ErrUnshared) {
		t.Fatalf("expected ErrUnshared, got %v", err)
	}
}

func TestPreviewUpdate(t *testing.T) {
	a, b := team(t)
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	a.Save("group audio", "fail")
	p, err := b.PreviewUpdate()
	if err != nil || p.Action != "fast-forward" || len(p.Changes) != 1 || len(p.Conflicts) != 0 {
		t.Fatalf("%v %+v", err, p)
	}
}
