package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/profile"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// The Rules window's switches write the fewest rules: leaving a folder out
// adds one, taking that back removes it; tracking what a preset leaves out
// adds a track rule.
func TestRulesWindow(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	fake := s3test.New("one")
	defer fake.Close()
	t.Cleanup(waitTidy)
	a := NewApp()
	team, _ := a.CreateStorageTeam(remote.Storage{Endpoint: fake.URL, Bucket: "one", AccessKey: "k", SecretKey: "s"}, "One")
	root := newSong(t)
	os.MkdirAll(filepath.Join(root, "Exports"), 0o755)
	os.MkdirAll(filepath.Join(root, "Backup"), 0o755)
	if _, err := a.AddProjectToTeam(team.ID, root); err != nil {
		t.Fatal(err)
	}
	d, err := a.ProjectRules(root)
	if err != nil || d.Error != "" || len(d.Presets) != 1 || d.Presets[0].Preset != "ableton" || !d.Presets[0].Found {
		t.Fatalf("rules: %+v %v", d, err)
	}
	nodes := func() map[string]RuleNode {
		ns, err := a.RulesFolder(root, "")
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]RuleNode{}
		for _, n := range ns {
			m[n.Name] = n
		}
		return m
	}
	if n := nodes(); n["Exports"].Ignored || !n["Backup"].Ignored || n["Song.als"].Size == 0 {
		t.Fatalf("nodes: %+v", n)
	}
	rules := func() []RuleItem { d, _ := a.ProjectRules(root); return d.Rules }

	if err := a.SetTracked(root, "Exports", true, false); err != nil {
		t.Fatal(err)
	}
	if r := rules(); len(r) != 1 || r[0] != (RuleItem{"ignore", "/Exports/"}) || !nodes()["Exports"].Ignored {
		t.Fatalf("leaving Exports out: %+v", r)
	}
	if err := a.SetTracked(root, "Exports", true, true); err != nil {
		t.Fatal(err)
	}
	if r := rules(); len(r) != 0 || nodes()["Exports"].Ignored {
		t.Fatalf("taking it back: %+v", r)
	}
	if err := a.SetTracked(root, "Backup", true, true); err != nil {
		t.Fatal(err)
	}
	if r := rules(); len(r) != 1 || r[0] != (RuleItem{"track", "/Backup/"}) || nodes()["Backup"].Ignored {
		t.Fatalf("tracking Backup: %+v", r)
	}
	if err := a.RemoveRule(root, 0); err != nil || len(rules()) != 0 {
		t.Fatalf("remove: %v", err)
	}
	if err := a.SetTracked(root, profile.FileName, false, false); err == nil {
		t.Error("the rules' file is always tracked")
	}
}
