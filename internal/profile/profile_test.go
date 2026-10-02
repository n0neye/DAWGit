package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func abletonFolder(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "Ableton Project Info"), 0o755)
	os.WriteFile(filepath.Join(root, "Song.als"), []byte("x"), 0o644)
	return root
}

// Without .dawgit.yaml an Ableton project behaves as DAWGit always did.
func TestDetectedAbleton(t *testing.T) {
	p := Detect(abletonFolder(t))
	if a := p.Applied(); len(a) != 1 || a[0].Preset != "ableton" || !a[0].Detected || a[0].Folder != "" {
		t.Fatalf("applied: %+v", a)
	}
	for _, c := range []struct {
		path    string
		dir     bool
		ignored bool
	}{
		{"Backup", true, true},
		{"Backup/Song [2026].als", false, true},
		{"Samples/Backup", true, false}, // only the project's own Backup folder
		{"Samples/kick.wav.asd", false, true},
		{"Samples/KICK.ASD", false, true},
		{".dawgit", true, true},
		{".dawgit/index.json", false, true},
		{"Samples/.git", true, true},
		{"Samples/desktop.ini", false, true},
		{"Samples/Thumbs.db", false, true},
		{"Art/cover.af~lock~", false, true}, // Affinity has the file open
		{"Art/cover.af", false, false},
		{".dawgit-tmp123", false, true},
		{"Song.als", false, false},
		{"Samples/kick.wav", false, false},
		{".dawgit.yaml", false, false},
	} {
		if got := p.Ignored(c.path, c.dir); got != c.ignored {
			t.Errorf("%s: ignored = %v (%s)", c.path, got, p.Explain(c.path, c.dir).By)
		}
	}
	if p.Kind("Song.als") != "set" || p.Kind("Samples/x.WAV") != "audio" || p.Kind("a.adg") != "live" ||
		p.Kind("x.mid") != "midi" || p.Kind("notes.txt") != "other" {
		t.Error("kinds")
	}
	if h := p.Handler("Song.als"); h.Merge != "ableton-set" || h.Samples != "ableton" {
		t.Errorf("handler: %+v", h)
	}
	if r := p.Running(); len(r) != 1 || r[0] != "ableton-live" {
		t.Errorf("running: %v", r)
	}
	if a := Detect(t.TempDir()).Applied(); len(a) != 0 {
		t.Errorf("empty folder detected as %+v", a)
	}
}

func TestRules(t *testing.T) {
	root := abletonFolder(t)
	p, err := Parse([]byte(`
requires: "0.7"
use:
  ./: ableton
rules:
  - ignore: "**/Exports/"
  - ignore: "*.tmp"
  - track: "**/*.asd"
  - ignore: "Samples/Recorded/*.asd"
  - ignore: "/Notes.txt"
`), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		path    string
		dir     bool
		ignored bool
		by      string
	}{
		{"Exports", true, true, "rule 1"},
		{"Mixes/Exports/final.wav", false, true, "rule 1"},
		{"Exports.wav", false, false, ""},
		{"a/b/x.tmp", false, true, "rule 2"},
		{"Samples/kick.asd", false, false, "rule 3"},
		{"Samples/Recorded/take.asd", false, true, "rule 4"},
		{"Notes.txt", false, true, "rule 5"},
		{"Sub/Notes.txt", false, false, ""},
		{"Backup/x.als", false, true, "preset ableton"},
		{".dawgit/x", false, true, "DAWGit"},
	} {
		d := p.Explain(c.path, c.dir)
		if d.Ignored != c.ignored || (c.by != "" && !strings.Contains(d.By, c.by)) {
			t.Errorf("%s: %+v", c.path, d)
		}
	}
	// A track rule can reach into an ignored folder, so it is not skipped.
	if p.SkipDir("Backup") {
		t.Error("skipped a folder a track rule may reach into")
	}
	if !p.SkipDir(".dawgit") {
		t.Error("scanned .dawgit")
	}
	if Detect(root).SkipDir("Backup") != true {
		t.Error("Backup not skipped without track rules")
	}
	if p.NeedsNewer("0.6.1") != "0.7" || p.NeedsNewer("0.7.0") != "" || p.NeedsNewer("1.0") != "" {
		t.Error("requires")
	}
}

func TestMixedFolders(t *testing.T) {
	root := t.TempDir()
	p, err := Parse([]byte("use:\n  Music/: ableton\n  Tools/: none\n"), root)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ignored("Music/Backup", true) || p.Ignored("Backup", true) || p.Ignored("Tools/Backup", true) {
		t.Error("a preset applies only inside its folder")
	}
	if p.Kind("Music/Song.als") != "set" || p.Kind("Song.als") != "other" {
		t.Error("kinds by folder")
	}
}

func TestErrors(t *testing.T) {
	root := abletonFolder(t)
	for _, c := range []struct{ yaml, want string }{
		{"rules:\n  - ignor: x\n", `line 2: unknown field "ignor"`}, // misspelt field
		{"rules:\n  - ignore: a\n    track: b\n", "either"},
		{"rules:\n  - {}\n", "either"},
		{"use:\n  ./: bitwig\n", "not a preset"},
		{"requires: soon\n", "requires"},
		{"rules:\n  - ignore: \"[\"\n", "bad pattern"},
		{"rules: [", ""},
	} {
		p, err := Parse([]byte(c.yaml), root)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: %v", c.yaml, err)
		}
		// Still usable: the detected profile.
		if p == nil || !p.Ignored("Backup", true) {
			t.Errorf("%q: no fallback", c.yaml)
		}
	}
	if p, err := Parse(nil, root); err != nil || !p.Ignored("Backup", true) {
		t.Errorf("empty file: %v", err)
	}
}

// The quick way to match "*.png" agrees with matchPath.
func TestEndsWithMatchesLikeMatchPath(t *testing.T) {
	pats := []string{"*.png", "*.PNG", " *.tar.gz", "*~", "*.c?", "*.[ch]", "*", "a*.png", "*/x.png", "*.meta"}
	paths := []string{"a.png", "Art/A.PNG", "x.tar.gz", "notes~", "f.cs", "f.h", "pics.png/readme.txt",
		"a", "dir/a.meta", ".png", "png", "x/y/z.Meta"}
	for _, pat := range pats {
		end, ok := endsWith(pat)
		if !ok {
			continue
		}
		for _, rel := range paths {
			quick := false
			for _, s := range strings.Split(strings.ToLower(rel), "/") {
				quick = quick || strings.HasSuffix(s, end)
			}
			if want := matchPath(pat, rel, false); quick != want {
				t.Errorf("%q on %q: quick %v, matchPath %v", pat, rel, quick, want)
			}
		}
	}
	for pat, ok := range map[string]bool{"*.png": true, "*.c?": false, "*": false, "a*.png": false, "*/x": false} {
		if _, got := endsWith(pat); got != ok {
			t.Errorf("endsWith(%q) = %v", pat, got)
		}
	}
}

// A Live project in a folder of a bigger project is found: its rules apply
// there, also with a .dawgit.yaml that doesn't say which presets to use.
func TestDetectsProjectsInside(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "Music", "Theme Project")
	os.MkdirAll(filepath.Join(live, "Ableton Project Info"), 0o755)
	os.MkdirAll(filepath.Join(live, "Backup"), 0o755)
	os.MkdirAll(filepath.Join(root, "Art"), 0o755)
	os.MkdirAll(filepath.Join(root, "Backup"), 0o755)
	check := func(p *Profile) {
		t.Helper()
		if !p.Ignored("Music/Theme Project/Backup", true) || p.Ignored("Backup", true) {
			t.Errorf("Live's rules inside its folder only: %+v", p.Applied())
		}
		if p.Kind("Music/Theme Project/Song.als") != "set" {
			t.Error("the set is a set")
		}
	}
	check(Detect(root))
	os.WriteFile(filepath.Join(root, FileName), []byte("rules:\n  - ignore: \"*.tmp\"\n"), 0o644)
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	check(p)
	// Deeper than DAWGit looks: not found.
	deep := filepath.Join(root, "a", "b", "c", "d")
	os.MkdirAll(filepath.Join(deep, "Ableton Project Info"), 0o755)
	if len(Detect(root).Applied()) != 1 {
		t.Errorf("too deep: %+v", Detect(root).Applied())
	}
}
