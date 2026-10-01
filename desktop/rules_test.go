package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/profile"
)

// The file DAWGit creates is valid as it is, and names the preset it found.
func TestRulesFile(t *testing.T) {
	song := t.TempDir()
	os.WriteFile(filepath.Join(song, "Song.als"), []byte("x"), 0o644)
	p, err := profile.Parse([]byte(rulesFile(song)), song)
	if err != nil {
		t.Fatal(err)
	}
	if a := p.Applied(); len(a) != 1 || a[0].Preset != "ableton" || a[0].Detected {
		t.Fatalf("applied: %+v", a)
	}
	if len(p.Rules) != 0 || !p.Ignored("Backup", true) {
		t.Fatalf("rules: %+v", p.Rules)
	}
	// A folder no preset knows: no use:, so detection goes on as before.
	plain := t.TempDir()
	if text := rulesFile(plain); strings.Contains(text, "use:") {
		t.Fatalf("plain folder:\n%s", text)
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
