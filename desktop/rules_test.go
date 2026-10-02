package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/profile"
	"dawgit/internal/project"
)

// A Live project added to a folder of the project is suggested, with what
// its preset would leave out; taking it (or saying none) ends the suggestion.
func TestRuleSuggestions(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, profile.FileName), []byte(profile.Generate(root)), 0o644)
	live := filepath.Join(root, "Music", "New Project")
	os.MkdirAll(filepath.Join(live, "Ableton Project Info"), 0o755)
	os.MkdirAll(filepath.Join(live, "Backup"), 0o755)
	os.WriteFile(filepath.Join(live, "Backup", "Song [old].als"), []byte("0123456789"), 0o644)
	os.WriteFile(filepath.Join(live, "Song.als"), []byte("x"), 0o644)
	r, err := project.Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	s := suggestions(r)
	if len(s) != 1 || s[0].Folder != "Music/New Project" || s[0].Preset != "ableton" || s[0].LeftOutBytes != 10 {
		t.Fatalf("suggestions %+v", s)
	}
	if err := r.SetPreset(s[0].Folder, s[0].Preset); err != nil {
		t.Fatal(err)
	}
	if s := suggestions(r); len(s) != 0 {
		t.Errorf("after taking it: %+v", s)
	}
}

func TestWithIgnoreRule(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"template": {"requires: \"0.8\"\nrules:\n  # Later rules win.\n  # - ignore: \"Exports/\"\n",
			"requires: \"0.8\"\nrules:\n  # Later rules win.\n  # - ignore: \"Exports/\"\n  - ignore: \"*.tmp\"\n"},
		"after rules": {"rules:\n- ignore: \"a/\"\n- track: \"b\"\nuse:\n  ./: ableton\n",
			"rules:\n- ignore: \"a/\"\n- track: \"b\"\n- ignore: \"*.tmp\"\nuse:\n  ./: ableton\n"},
		"empty list": {"rules: []\n", "rules:\n  - ignore: \"*.tmp\"\n"},
		"no rules":   {"requires: \"0.8\"\n\n", "requires: \"0.8\"\nrules:\n  - ignore: \"*.tmp\"\n"},
		"crlf":       {"rules:\r\n  - ignore: \"a/\"\r\n", "rules:\r\n  - ignore: \"a/\"\r\n  - ignore: \"*.tmp\"\r\n"},
		"there":      {"rules:\n  - ignore: \"*.tmp\"\n", "rules:\n  - ignore: \"*.tmp\"\n"},
	} {
		got, err := withIgnoreRule(c.in, "*.tmp")
		if err != nil || got != c.want {
			t.Errorf("%s: got\n%q\nwant\n%q (%v)", name, got, c.want, err)
		}
		if _, err := profile.Parse([]byte(got), t.TempDir()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The options offered match what they say, even for names with [ or *.
func TestIgnoreOptions(t *testing.T) {
	a := &App{}
	file := a.IgnoreOptions("Backup/Song [old].als", false)
	if len(file) != 2 || file[1].Pattern != "*.als" {
		t.Fatalf("file options: %+v", file)
	}
	dir := a.IgnoreOptions("Renders/Final", true)
	if len(dir) != 2 || dir[0].Pattern != "/Renders/Final/" || dir[1].Pattern != "Final/" {
		t.Fatalf("folder options: %+v", dir)
	}
	text, _ := withIgnoreRule("", file[0].Pattern)
	p, err := profile.Parse([]byte(text), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ignored("Backup/Song [old].als", false) || p.Ignored("Backup/Song o.als", false) {
		t.Fatalf("this file only: %s", text)
	}
	if len(a.IgnoreOptions(".dawgit.yaml", false)) != 0 {
		t.Fatal(".dawgit.yaml can't be ignored")
	}
}
