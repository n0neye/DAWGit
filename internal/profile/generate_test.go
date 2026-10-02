package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A folder of a bigger project: art, a Live project inside.
func studioFolder(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Music", "Theme Project", "Ableton Project Info"), 0o755)
	os.MkdirAll(filepath.Join(root, "Art"), 0o755)
	return root
}

// The generated file names what DAWGit found, marked, and reads back as
// the same rules; the same folder always gives the same text.
func TestGenerate(t *testing.T) {
	root := studioFolder(t)
	text := Generate(root)
	if text != Generate(root) {
		t.Fatal("not the same text twice")
	}
	if !strings.Contains(text, `"Music/Theme Project/": ableton  `+FoundMark) || !strings.Contains(text, `requires: "`+PresetsVersion+`"`) {
		t.Fatalf("generated:\n%s", text)
	}
	p, err := Parse([]byte(text), root)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ignored("Music/Theme Project/Backup", true) || len(p.Suggestions()) != 0 {
		t.Errorf("rules %+v, suggestions %+v", p.Applied(), p.Suggestions())
	}
	// Nothing found: the project folder gets none, so later finds are suggested.
	if text := Generate(t.TempDir()); !strings.Contains(text, "  ./: none\n") {
		t.Errorf("empty folder:\n%s", text)
	}
}

// A Live project added later is suggested, not applied; a folder the file
// names (even as none) is not suggested again.
func TestSuggestions(t *testing.T) {
	root := t.TempDir()
	text := Generate(root)
	os.MkdirAll(filepath.Join(root, "Music", "New Project", "Ableton Project Info"), 0o755)
	p, err := Parse([]byte(text), root)
	if err != nil {
		t.Fatal(err)
	}
	s := p.Suggestions()
	if len(s) != 1 || s[0].Folder != "Music/New Project" || s[0].Preset != "ableton" || len(s[0].LeftOut) == 0 {
		t.Fatalf("suggestions %+v", s)
	}
	if p.Ignored("Music/New Project/Backup", true) {
		t.Error("a suggestion is not applied")
	}
	for _, preset := range []string{"ableton", "none"} {
		changed, err := SetPreset(text, "Music/New Project", preset, false)
		if err != nil {
			t.Fatal(err)
		}
		p, err := Parse([]byte(changed), root)
		if err != nil {
			t.Fatalf("%v\n%s", err, changed)
		}
		if len(p.Suggestions()) != 0 || p.Ignored("Music/New Project/Backup", true) != (preset == "ableton") {
			t.Errorf("%s:\n%s", preset, changed)
		}
	}
	// Without presets: (an older file) detection applies: nothing to suggest.
	p, _ = Parse([]byte("rules:\n  - ignore: \"*.tmp\"\n"), root)
	if len(p.Suggestions()) != 0 {
		t.Error("detected rules suggest nothing")
	}
}

func TestSetPreset(t *testing.T) {
	for name, c := range map[string]struct{ in, want string }{
		"replace, keeping comments": {"presets:\n  ./: design  # found by DAWGit\n  \"Game/\": none\nrules:\n",
			"presets:\n  ./: design  # found by DAWGit\n  \"Game/\": unity\nrules:\n"},
		"add at the end": {"presets:\n  ./: design\n\nrules:\n", "presets:\n  ./: design\n  \"Game/\": unity\n\nrules:\n"},
		"older use:":     {"use:\n  ./: design\n", "presets:\n  ./: design\n  \"Game/\": unity\n"},
		"no presets":     {"rules:\n  - ignore: \"a/\"\n", "presets:\n  \"Game/\": unity\nrules:\n  - ignore: \"a/\"\n"},
		"crlf":           {"presets:\r\n  ./: design\r\n", "presets:\r\n  ./: design\r\n  \"Game/\": unity\r\n"},
	} {
		got, err := SetPreset(c.in, "Game", "unity", false)
		if err != nil || got != c.want {
			t.Errorf("%s: %v\n%q\nwant\n%q", name, err, got, c.want)
		}
	}
	if _, err := Parse([]byte("presets:\n  ./: ableton\nuse:\n  ./: ableton\n"), t.TempDir()); err == nil {
		t.Error("presets: and use: together")
	}
}
