package agent

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/server"
)

var fixtures = filepath.Join("..", "..", "SampleProjects", "SampleAbletonProject Project")

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	io.Copy(out, in)
}

func kinds(events []Event) map[EventKind]Event {
	out := map[EventKind]Event{}
	for _, e := range events {
		out[e.Kind] = e
	}
	return out
}

func TestWatcherEvents(t *testing.T) {
	st, _ := server.OpenStorage(t.TempDir())
	srv := httptest.NewServer(server.Handler(st, "tok"))
	defer srv.Close()

	rootA := filepath.Join(t.TempDir(), "Song Project")
	copyFile(t, filepath.Join(fixtures, "SampleAbletonProject_v2.als"), filepath.Join(rootA, "Song.als"))
	a, err := project.Init(rootA, "yi")
	if err != nil {
		t.Fatal(err)
	}
	a.SetRemote(srv.URL, "tok")
	if _, _, err := a.Save("v2", project.Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := project.Clone(srv.URL, "tok", "Song", filepath.Join(t.TempDir(), "B"), "alex")
	if err != nil {
		t.Fatal(err)
	}

	w := New(b.Root)
	if ev := w.Check(); len(ev) != 0 {
		t.Fatalf("quiet start expected, got %+v", ev)
	}

	// Yi edits in Live: Alex's watcher reports it.
	copyFile(t, filepath.Join(fixtures, "Split-A.als"), filepath.Join(rootA, "Song.als"))
	a.ReportWorkspace()
	ev := kinds(w.Check())
	if e, ok := ev[TeammateEditing]; !ok || e.Author != "yi" || len(e.Labels) == 0 {
		t.Fatalf("expected teammate editing, got %+v", ev)
	}

	// Alex edits the same tracks: backup + overlap warning (once).
	copyFile(t, filepath.Join(fixtures, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	ev = kinds(w.Check())
	if _, ok := ev[BackedUp]; !ok {
		t.Errorf("expected backup, got %+v", ev)
	}
	if _, ok := ev[Overlap]; !ok {
		t.Errorf("expected overlap, got %+v", ev)
	}
	if ev := kinds(w.Check()); len(ev) != 0 {
		t.Errorf("events repeated: %+v", ev)
	}

	// Yi saves: new version, and Yi is no longer editing.
	a.Save("group audio", project.Strategy("fail"))
	a.ReportWorkspace()
	ev = kinds(w.Check())
	if e, ok := ev[NewVersions]; !ok || len(e.Versions) != 1 || e.Versions[0].Message != "group audio" || e.Waiting {
		t.Errorf("expected one new version, got %+v", ev)
	}
	if _, ok := ev[TeammateIdle]; !ok {
		t.Errorf("expected teammate idle, got %+v", ev)
	}
}
