package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/profile"
)

func statusOf(t *testing.T, r *Repo) map[string]string {
	t.Helper()
	changes, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, c := range changes {
		out[c.Path] = c.Status
	}
	return out
}

// Ignoring a folder takes it out of versions, but never deletes it from
// anyone's computer; tracking it again brings it back into versions.
func TestIgnoreRuleKeepsFilesOnDisk(t *testing.T) {
	a, b := team(t)
	os.MkdirAll(a.Abs("Exports"), 0o755)
	os.WriteFile(a.Abs("Exports/mix.wav"), []byte("RIFF-mix"), 0o644)
	if _, _, err := a.Save("a mix", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}

	// Yi starts ignoring Exports/.
	os.WriteFile(a.Abs(profile.FileName), []byte("rules:\n  - ignore: \"Exports/\"\n"), 0o644)
	a.forgetProfile()
	st := statusOf(t, a)
	if st["Exports/mix.wav"] != "untracked" || st[profile.FileName] != "added" {
		t.Fatalf("status: %v", st)
	}
	if _, _, err := a.Save("ignore exports", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	m, _ := a.Load(a.Head())
	if _, ok := m.FileMap()["Exports/mix.wav"]; ok {
		t.Fatal("ignored file still in the version")
	}
	assertClean(t, a)

	// Alex takes it in: the mix stays on Alex's computer, now untracked.
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(b.Abs("Exports/mix.wav")); err != nil || string(got) != "RIFF-mix" {
		t.Fatalf("Alex's mix: %q %v", got, err)
	}
	assertClean(t, b)

	// Tracking it again: back in the next version.
	os.WriteFile(a.Abs(profile.FileName), []byte("rules:\n  - ignore: \"Exports/\"\n  - track: \"Exports/mix.wav\"\n"), 0o644)
	a.forgetProfile()
	if st := statusOf(t, a); st["Exports/mix.wav"] != "added" {
		t.Fatalf("status after track: %v", st)
	}
	if _, _, err := a.Save("track the mix", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	assertClean(t, b)
}

func TestRulesThatCantBeFollowed(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	mustSnapshot(t, r, "first")

	// A broken file: the project is still readable, committing is refused.
	os.WriteFile(r.Abs(profile.FileName), []byte("rules:\n  - ignor: x\n"), 0o644)
	r.forgetProfile()
	if _, err := r.Status(); err != nil {
		t.Fatalf("status with a broken file: %v", err)
	}
	if _, err := r.Snapshot("x"); err == nil || !strings.Contains(err.Error(), "fix the project's rules") {
		t.Fatalf("commit with a broken file: %v", err)
	}

	// A file for a newer DAWGit.
	os.WriteFile(r.Abs(profile.FileName), []byte("requires: \"99.0\"\n"), 0o644)
	r.forgetProfile()
	if _, err := r.Snapshot("x"); err == nil || !strings.Contains(err.Error(), "needs DAWGit 99.0") {
		t.Fatalf("commit with a newer file: %v", err)
	}
	// Taking in a version committed with such a file is refused too.
	h, n, _ := r.Store.Put(strings.NewReader("requires: \"99.0\"\n"))
	if _, err := r.profileOf(&Manifest{Files: []FileEntry{{Path: profile.FileName, Hash: h, Size: n}}}); err == nil {
		t.Fatal("a version needing a newer DAWGit was accepted")
	}
	os.Remove(filepath.Join(root, profile.FileName))
}

// The rules file is written when missing and given presets: when older;
// the second time nothing changes.
func TestEnsureRules(t *testing.T) {
	r, _ := Init(newProject(t), "yi")
	if did, err := r.EnsureRules(); err != nil || did != "created" {
		t.Fatalf("first: %q %v", did, err)
	}
	p, err := r.Profile()
	if err != nil || !p.FromFile || p.Named == nil || !p.Ignored("Backup", true) {
		t.Fatalf("rules from the file: %+v %v", p.Applied(), err)
	}
	if did, _ := r.EnsureRules(); did != "" {
		t.Errorf("again: %q", did)
	}
	// An older file: rules only.
	os.WriteFile(r.Abs(profile.FileName), []byte("requires: \"0.7\"\nrules:\n  - ignore: \"Exports/\"\n"), 0o644)
	if did, err := r.EnsureRules(); err != nil || did != "presets added" {
		t.Fatalf("older file: %q %v", did, err)
	}
	data, _ := os.ReadFile(r.Abs(profile.FileName))
	text := string(data)
	if !strings.Contains(text, "./: ableton  "+profile.FoundMark) || !strings.Contains(text, `requires: "`+profile.PresetsVersion+`"`) ||
		!strings.Contains(text, `- ignore: "Exports/"`) {
		t.Errorf("older file now:\n%s", text)
	}
	// A broken file is the person's to fix.
	os.WriteFile(r.Abs(profile.FileName), []byte("rules: [\n"), 0o644)
	if did, _ := r.EnsureRules(); did != "" {
		t.Errorf("broken file changed: %q", did)
	}
}
