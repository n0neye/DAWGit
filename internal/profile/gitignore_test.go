package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitignore(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "# build output\nbuild/\n*.log\n!keep.log\n/secret.txt\n\\#hash.txt\ntrailing.txt   \n")
	write(t, root, "web/.gitignore", "node_modules/\n!*.log\n/dist\n")
	write(t, root, FileName, "gitignore: true\n")
	p, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for rel, ignored := range map[string]bool{
		"build/x.o": true, "src/build/y.o": true, // build/ anywhere
		"app.log": true, "keep.log": false, // ! takes it back
		"secret.txt": true, "docs/secret.txt": false, // anchored to the folder of the .gitignore
		"#hash.txt": true, "trailing.txt": true,
		"web/node_modules/react/index.js": true,
		"web/debug.log":                   false, // the deeper .gitignore wins
		"web/dist/app.js":                 true, "dist/app.js": false,
		"src/main.go": false, ".gitignore": false,
	} {
		if got := p.Ignored(rel, false); got != ignored {
			t.Errorf("%s: ignored = %v (%s)", rel, got, p.Explain(rel, false).By)
		}
	}
	if !p.SkipDir("web/node_modules") || p.SkipDir("web") {
		t.Error("node_modules should be skipped, web not")
	}
	if by := p.Explain("app.log", false).By; by != ".gitignore line 3: *.log" {
		t.Errorf("explained as %q", by)
	}
	// .dawgit.yaml rules win over .gitignore.
	write(t, root, FileName, "gitignore: true\nrules:\n  - track: \"*.log\"\n")
	p, _ = Load(root)
	if p.Ignored("app.log", false) {
		t.Error("a track rule should win")
	}
	// Without gitignore: true, .gitignore files are not read.
	write(t, root, FileName, "rules: []\n")
	p, _ = Load(root)
	if p.Ignored("app.log", false) {
		t.Error("gitignore off")
	}
}

// A fallback preset is detected only when no other one is.
func TestFallbackPreset(t *testing.T) {
	if err := RegisterPreset([]byte("name: zz-code\nfallback: true\ngitignore: true\ndetect: [\".gitignore\"]\n")); err != nil {
		t.Fatal(err)
	}
	defer delete(builtin, "zz-code")
	plain := t.TempDir()
	write(t, plain, ".gitignore", "*.tmp\n")
	p := Detect(plain)
	if a := p.Applied(); len(a) != 1 || a[0].Preset != "zz-code" || !p.Gitignore || !p.Ignored("x.tmp", false) {
		t.Fatalf("plain folder: %+v", a)
	}
	song := t.TempDir()
	write(t, song, ".gitignore", "*.tmp\n")
	write(t, song, "Ableton Project Info/x.cfg", "")
	if a := Detect(song).Applied(); len(a) != 1 || a[0].Preset != "ableton" {
		t.Fatalf("a Live project with a .gitignore: %+v", a)
	}
}
