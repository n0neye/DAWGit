package ext_test

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/ext"
	"dawgit/internal/handlers"
	"dawgit/internal/profile"
	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/server"
)

func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "dawgit-ext-")
	os.Setenv("DAWGIT_CONFIG_DIR", dir) // not the user's real teams
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// A made-up tool, as an extension would add one.
const notesPreset = `
name: notes
detect: ["notes.project"]
ignore: ["/Cache/"]
handlers:
  - files: ["*.txt"]
    merge: lines
running: notes-app
kinds:
  note: ["*.txt"]
`

// mergeLines keeps both sides' added lines (enough for the test).
func mergeLines(base, ours, theirs []byte) ([]byte, bool, error) {
	seen := map[string]bool{}
	var out []string
	for _, b := range [][]byte{ours, theirs} {
		for _, l := range strings.SplitAfter(string(b), "\n") {
			if l != "" && !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	if bytes.Contains(ours, []byte("CLASH")) && bytes.Contains(theirs, []byte("CLASH")) {
		return nil, false, nil
	}
	return []byte(strings.Join(out, "")), true, nil
}

func TestExtension(t *testing.T) {
	if err := ext.RegisterPreset([]byte(notesPreset)); err != nil {
		t.Fatal(err)
	}
	ext.RegisterMerge("lines", mergeLines)
	ext.RegisterRunning("notes-app", func(root string) string { return "" })

	// The tool's project is recognized: it can be tracked, its cache is not.
	root := filepath.Join(t.TempDir(), "Book")
	os.MkdirAll(filepath.Join(root, "Cache"), 0o755)
	os.WriteFile(filepath.Join(root, "notes.project"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "Cache", "big.bin"), []byte("cache"), 0o644)
	os.WriteFile(filepath.Join(root, "chapter.txt"), []byte("one\n"), 0o644)
	p := profile.Detect(root)
	if a := p.Applied(); len(a) != 1 || a[0].Preset != "notes" || p.Kind("chapter.txt") != "note" ||
		!p.Ignored("Cache/big.bin", false) || handlers.Running("notes-app") == nil {
		t.Fatalf("preset not in effect: %+v", a)
	}
	a, err := project.Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}

	// Two people change the same text file: merged by the extension's handler.
	st, _ := server.OpenStorage(t.TempDir())
	srv := httptest.NewServer(server.Handler(st, "tok"))
	defer srv.Close()
	if err := a.SetRemote(srv.URL, "tok"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.Save("start", project.Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := project.Clone(srv.URL, "tok", "Book", filepath.Join(t.TempDir(), "B"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(a.Abs("chapter.txt"), []byte("one\ntwo\n"), 0o644)
	os.WriteFile(b.Abs("chapter.txt"), []byte("one\nthree\n"), 0o644)
	if _, _, err := a.Save("two", project.Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if _, res, err := b.Save("three", project.Strategy("fail")); err != nil || res.Action != "published" {
		t.Fatalf("save with a handler merge: %v %+v", err, res)
	}
	if got, _ := os.ReadFile(b.Abs("chapter.txt")); string(got) != "one\nthree\ntwo\n" {
		t.Fatalf("merged: %q", got)
	}

	// Colliding changes fall back to choosing the whole file.
	os.WriteFile(a.Abs("chapter.txt"), []byte("CLASH a\n"), 0o644)
	os.WriteFile(b.Abs("chapter.txt"), []byte("CLASH b\n"), 0o644)
	a.Update(project.Strategy("fail"))
	a.Save("a clash", project.Strategy("fail"))
	var conflict *project.MergeConflictError
	if _, _, err := b.Save("b clash", project.Strategy("fail")); !errors.As(err, &conflict) {
		t.Fatalf("colliding change: %v", err)
	}
}

// A backend kind added by an extension is used for its addresses.
func TestBackendRegistration(t *testing.T) {
	var opened string
	ext.RegisterBackend("memo+", func(cfg ext.Config) (ext.Backend, error) {
		opened = cfg.URL
		return remote.New("http://127.0.0.1:1", ""), nil
	})
	if _, err := remote.Open(remote.Config{URL: "memo+team://x"}); err != nil || opened != "memo+team://x" {
		t.Fatalf("opened %q, %v", opened, err)
	}
	if c := remote.CapabilitiesOf(remote.New("http://x", "")); c.Locks || c.Presence {
		t.Error("built-in backends claim capabilities")
	}
}
