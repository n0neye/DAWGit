package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// undoTeam: A shared v1 (a, b), then a wrong commit (a changed, c added),
// then a teammate changed b on top of it. A is up to date.
func undoTeam(t *testing.T) (a *Repo, code, wrong string) {
	t.Helper()
	fake := s3test.New("team")
	t.Cleanup(fake.Close)
	code = remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ = Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "a.txt", "a1")
	write(t, a.Root, "b.txt", "b1")
	if _, _, err := a.Save("v1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "a.txt", "a wrong")
	write(t, a.Root, "c.txt", "should not be here")
	m, _, err := a.Save("wrong", Strategy("fail"))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", a.Config.Name, filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	write(t, b.Root, "b.txt", "b by the teammate")
	if _, _, err := b.Save("teammate", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	return a, code, m.ID
}

// Undoing a version takes back its changes and keeps what came after it.
func TestUndoCommit(t *testing.T) {
	a, code, wrong := undoTeam(t)
	write(t, a.Root, "notes.txt", "uncommitted, elsewhere")
	plan, err := a.PlanUndo(wrong, Strategy("fail"))
	if err != nil || len(plan.Changed) != 2 || len(plan.Blocked) != 0 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if _, _, err := a.UndoCommit(wrong, "", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	got := files(t, a.Root)
	if got["a.txt"] != "a1" || got["b.txt"] != "b by the teammate" || got["notes.txt"] != "uncommitted, elsewhere" {
		t.Fatalf("files: a %q b %q notes %q", got["a.txt"], got["b.txt"], got["notes.txt"])
	}
	if _, ok := got["c.txt"]; ok {
		t.Fatal("c.txt still there")
	}
	// The uncommitted change stays uncommitted; the undo is shared.
	if ch, _ := a.Status(); len(ch) != 1 || ch[0].Path != "notes.txt" {
		t.Fatalf("uncommitted after: %+v", ch)
	}
	c := cloneFiles(t, code, a.Config.Name)
	if c["a.txt"] != "a1" || c["b.txt"] != "b by the teammate" || c["c.txt"] != "" || c["notes.txt"] != "" {
		t.Fatalf("a teammate gets: %v", fileNames(c))
	}
	if _, _, err := a.UndoCommit(wrong, "", Strategy("fail")); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("again: %v", err)
	}
}

// Uncommitted changes in a file the undo changes stop it.
func TestUndoStopsForChangesInTheWay(t *testing.T) {
	a, _, wrong := undoTeam(t)
	write(t, a.Root, "a.txt", "editing it now")
	var touch *ErrUndoTouchesChanges
	if _, _, err := a.UndoCommit(wrong, "", Strategy("fail")); !errors.As(err, &touch) || touch.Paths[0] != "a.txt" {
		t.Fatalf("undo: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(a.Root, "a.txt")); string(got) != "editing it now" {
		t.Fatalf("the change was touched: %q", got)
	}
	if plan, _ := a.PlanUndo(wrong, Strategy("fail")); len(plan.Blocked) != 1 {
		t.Fatalf("plan: %+v", plan)
	}
}

// A later version that changed the same file is a conflict to decide.
func TestUndoConflict(t *testing.T) {
	a, _, wrong := undoTeam(t)
	write(t, a.Root, "a.txt", "a changed again later")
	if _, _, err := a.Save("later", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	var mc *MergeConflictError
	if _, _, err := a.UndoCommit(wrong, "", Strategy("fail")); !errors.As(err, &mc) {
		t.Fatalf("undo: %v", err)
	}
	// Deciding for the later change: only c.txt goes.
	if _, _, err := a.UndoCommit(wrong, "", Strategy("ours")); err != nil {
		t.Fatal(err)
	}
	got := files(t, a.Root)
	if got["a.txt"] != "a changed again later" || got["c.txt"] != "" {
		t.Fatalf("files: a %q c %q", got["a.txt"], got["c.txt"])
	}
}
