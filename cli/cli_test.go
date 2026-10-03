package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// Tests must not touch the user's real team store.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dawgit-cli-")
	if err != nil {
		panic(err)
	}
	os.Setenv("DAWGIT_CONFIG_DIR", dir)
	openSet = func(string) string { return "" } // Live is not running
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type reply struct {
	Schema  int             `json:"schema"`
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Result  json.RawMessage `json:"result"`
	Error   *errorJSON      `json:"error"`
}

// run runs dawgit in dir with --json; returns the exit status and the reply.
func run(t *testing.T, dir string, args ...string) (int, reply) {
	t.Helper()
	t.Chdir(dir)
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() { b, _ := io.ReadAll(r); done <- b }()
	code := Run(append(args, "--json"))
	w.Close()
	os.Stdout = stdout
	out := <-done
	var rep reply
	if err := json.Unmarshal(out, &rep); err != nil {
		t.Fatalf("dawgit %s: not one JSON object: %v\n%s", strings.Join(args, " "), err, out)
	}
	if rep.Schema != 1 || rep.OK != (code == 0) || (rep.Error != nil && rep.Error.Exit != code) {
		t.Fatalf("dawgit %s: envelope %+v, exit %d", strings.Join(args, " "), rep, code)
	}
	return code, rep
}

func newFolder(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Song Project")
	os.MkdirAll(filepath.Join(dir, "Ableton Project Info"), 0o755)
	for p, c := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		os.WriteFile(filepath.Join(dir, p), []byte(c), 0o644)
	}
	return dir
}

func TestJSON(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a := newFolder(t, map[string]string{"Samples/kick.wav": "kick", "notes.txt": "first"})
	if _, err := project.Init(a, "yi"); err != nil {
		t.Fatal(err)
	}

	// Before anything is saved.
	exit, rep := run(t, a, "status")
	var st statusJSON
	json.Unmarshal(rep.Result, &st)
	if exit != 0 || st.Version != "" || st.Team != nil || len(st.Changes) == 0 {
		t.Fatalf("status: %s", rep.Result)
	}
	exit, rep = run(t, a, "save", "-m", "first")
	var sv syncJSON
	json.Unmarshal(rep.Result, &sv)
	if exit != 0 || sv.Action != "local" || sv.Saved == nil || sv.Saved.Message != "first" {
		t.Fatalf("save without a team: %s %+v", rep.Result, rep.Error)
	}

	// In a team, with a teammate.
	if exit, rep := run(t, a, "remote", code); exit != 2 || rep.Error.Code != "usage" {
		t.Fatalf("remote has no --json: %+v", rep)
	}
	ra, _ := project.Open(a)
	if err := ra.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	exit, rep = run(t, a, "save", "-m", "shared")
	json.Unmarshal(rep.Result, &sv)
	if exit != 0 || sv.Action != "published" {
		t.Fatalf("save: %s", rep.Result)
	}
	b := filepath.Join(t.TempDir(), "B", "Song Project")
	if _, _, err := project.Clone(code, "", ra.Config.Name, b, "alex"); err != nil {
		t.Fatal(err)
	}
	exit, rep = run(t, a, "log", "-n", "1")
	var lg struct{ Versions []versionJSON }
	json.Unmarshal(rep.Result, &lg)
	if exit != 0 || len(lg.Versions) != 1 || lg.Versions[0].Message != "first" || len(lg.Versions[0].Branches) != 1 {
		t.Fatalf("log: %s", rep.Result)
	}

	// Both change notes.txt: B is asked.
	os.WriteFile(filepath.Join(a, "notes.txt"), []byte("A's notes"), 0o644)
	run(t, a, "save", "-m", "A's notes")
	os.WriteFile(filepath.Join(b, "notes.txt"), []byte("B's notes"), 0o644)
	exit, rep = run(t, b, "status")
	json.Unmarshal(rep.Result, &st)
	if exit != 0 || st.Team == nil || !st.Team.Reachable || !st.Team.Incoming || len(st.Changes) != 1 ||
		st.Changes[0].Status != "modified" {
		t.Fatalf("status with the team ahead: %s", rep.Result)
	}
	// Uncommitted work is kept through an update, unless both changed a file.
	if exit, rep := run(t, b, "update"); exit != 3 || rep.Error.Code != "merge_conflict" {
		t.Fatalf("update with changes: %d %+v", exit, rep.Error)
	}
	exit, rep = run(t, b, "update", "--preview")
	var pv previewJSON
	json.Unmarshal(rep.Result, &pv)
	if exit != 0 || len(pv.Versions) != 1 || len(pv.Changes) != 1 {
		t.Fatalf("preview: %s", rep.Result)
	}
	exit, rep = run(t, b, "save", "-m", "B's notes")
	if exit != 3 || rep.Error.Code != "merge_conflict" || len(rep.Error.Conflicts) != 1 ||
		rep.Error.Conflicts[0].File != "notes.txt" || !strings.Contains(rep.Error.Hint, "--strategy") {
		t.Fatalf("conflict: %d %+v", exit, rep.Error)
	}

	// The set is open in Live: B is told, nothing changes.
	openSet = func(string) string { return "Song.als" }
	exit, rep = run(t, b, "save", "-m", "B's notes", "--strategy", "theirs")
	openSet = func(string) string { return "" }
	if exit != 4 || rep.Error.Code != "set_open_in_live" || rep.Error.Set != "Song.als" {
		t.Fatalf("Live open: %d %+v", exit, rep.Error)
	}
	exit, rep = run(t, b, "save", "-m", "B's notes", "--strategy", "theirs")
	json.Unmarshal(rep.Result, &sv)
	// Theirs taken: nothing of B's left to commit, B is just up to date.
	if exit != 0 || sv.Action != "fast-forward" || sv.Saved != nil || len(sv.Merged) == 0 {
		t.Fatalf("save with theirs: %s", rep.Result)
	}
	if got, _ := os.ReadFile(filepath.Join(b, "notes.txt")); string(got) != "A's notes" {
		t.Errorf("theirs kept %q", got)
	}
}

func TestErrors(t *testing.T) {
	empty := t.TempDir()
	if exit, rep := run(t, empty, "status"); exit != 6 || rep.Error.Code != "not_a_project" {
		t.Fatalf("not a project: %d %+v", exit, rep.Error)
	}
	if exit, rep := run(t, empty, "save", "--bogus"); exit != 2 || rep.Error.Code != "usage" {
		t.Fatalf("unknown flag: %d %+v", exit, rep.Error)
	}
	if exit, rep := run(t, empty, "save"); exit != 2 || rep.Error.Code != "usage" {
		t.Fatalf("no message: %d %+v", exit, rep.Error)
	}
	if exit, rep := run(t, empty, "frobnicate"); exit != 2 || rep.Error.Code != "usage" {
		t.Fatalf("unknown command: %d %+v", exit, rep.Error)
	}
	if exit, rep := run(t, empty, "version"); exit != 0 || !strings.Contains(string(rep.Result), `"version"`) {
		t.Fatalf("version: %d %s", exit, rep.Result)
	}
}

func TestEditPath(t *testing.T) {
	id := func(s string) string { return strings.ReplaceAll(s, "%LOCALAPPDATA%", `C:\Users\yi\AppData\Local`) }
	bin := `C:\Users\yi\AppData\Local\Programs\DAWGit\bin`
	for _, c := range []struct {
		in   string
		add  bool
		want string
		ch   bool
	}{
		{"", true, bin, true},
		{`C:\Go\bin;C:\tools`, true, `C:\Go\bin;C:\tools;` + bin, true},
		{`C:\Go\bin;`, true, `C:\Go\bin;` + bin, true},
		{`C:\Go\bin;%LOCALAPPDATA%\Programs\DAWGit\bin\`, true, `C:\Go\bin;%LOCALAPPDATA%\Programs\DAWGit\bin\`, false},
		{`C:\Go\bin;c:\users\YI\appdata\local\programs\dawgit\BIN;C:\tools`, false, `C:\Go\bin;C:\tools`, true},
		{`C:\Go\bin;%LOCALAPPDATA%\Programs\DAWGit\bin`, false, `C:\Go\bin`, true},
		{`C:\Go\bin;C:\Users\yi\AppData\Local\Programs\DAWGit Pro\bin`, false, `C:\Go\bin;C:\Users\yi\AppData\Local\Programs\DAWGit Pro\bin`, false},
	} {
		got, ch := editPath(c.in, bin, c.add, id)
		if got != c.want || ch != c.ch {
			t.Errorf("editPath(%q, add=%v) = %q %v, want %q %v", c.in, c.add, got, ch, c.want, c.ch)
		}
	}
}
