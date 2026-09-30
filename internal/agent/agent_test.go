package agent

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
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
	watchTeam(t, srv.URL, "tok")
}

// On storage the watcher reads the branch head and nothing else: no
// listings, which storage bills more.
func TestWatcherOnStorage(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	cfg, err := remote.Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k", SecretKey: "s"}.Config()
	if err != nil {
		t.Fatal(err)
	}
	root := watchTeam(t, remote.EncodeConnectionCode(cfg), "")
	lists := fake.Requests["LIST"]
	w := New(root)
	for i := 0; i < 3; i++ {
		w.Check()
	}
	if fake.Requests["LIST"] != lists {
		t.Errorf("polling listed the bucket %d times", fake.Requests["LIST"]-lists)
	}
}

// watchTeam has Yi commit while Alex's watcher looks on; it returns Alex's
// project folder.
func watchTeam(t *testing.T, address, token string) string {
	t.Helper()
	rootA := filepath.Join(t.TempDir(), "Song Project")
	copyFile(t, filepath.Join(fixtures, "SampleAbletonProject_v2.als"), filepath.Join(rootA, "Song.als"))
	a, err := project.Init(rootA, "yi")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetRemote(address, token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Save("v2", project.Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := project.Clone(address, token, "Song", filepath.Join(t.TempDir(), "B"), "alex")
	if err != nil {
		t.Fatal(err)
	}

	w := New(b.Root)
	if ev := w.Check(); len(ev) != 0 {
		t.Fatalf("quiet start expected, got %+v", ev)
	}
	// Edits in Live are nobody else's business now: no events.
	copyFile(t, filepath.Join(fixtures, "Split-A.als"), filepath.Join(rootA, "Song.als"))
	if ev := w.Check(); len(ev) != 0 {
		t.Fatalf("unsaved edits reported: %+v", ev)
	}

	// Yi commits: one new version, reported once.
	if _, _, err := a.Save("group audio", project.Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	ev := kinds(w.Check())
	if e, ok := ev[NewVersions]; !ok || len(e.Versions) != 1 || e.Versions[0].Message != "group audio" || e.Waiting {
		t.Errorf("expected one new version, got %+v", ev)
	}
	if ev := w.Check(); len(ev) != 0 {
		t.Errorf("events repeated: %+v", ev)
	}
	return b.Root
}
