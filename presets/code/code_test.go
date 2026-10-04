package code

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/ext"
	_ "dawgit/presets/unity"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(content), 0o644)
}

func TestCodePreset(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", "{}")
	write(t, root, ".gitignore", "dist/\n")
	rules, err := ext.LoadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if a := rules.Applied(); len(a) != 1 || a[0].Preset != "code" {
		t.Fatalf("applied: %+v", a)
	}
	for rel, ignored := range map[string]bool{
		"node_modules/react/index.js": true, "dist/app.js": true, ".env": true, ".env.example": false,
		"src/main.ts": false, "package.json": false, "web/__pycache__/x.pyc": true,
	} {
		if got := rules.Ignored(rel, false); got != ignored {
			t.Errorf("%s: ignored = %v", rel, got)
		}
	}
	if rules.Handler("src/main.ts").Merge != "text" || rules.Handler("logo.png").Merge != "" {
		t.Error("handlers")
	}
	// A Unity project with a .gitignore is a Unity project.
	unity := t.TempDir()
	write(t, unity, ".gitignore", "Library/\n")
	write(t, unity, "ProjectSettings/ProjectVersion.txt", "m_EditorVersion: 6000.0.1f1\n")
	u, _ := ext.LoadRules(unity)
	if a := u.Applied(); len(a) != 1 || a[0].Preset != "unity" {
		t.Fatalf("unity with .gitignore: %+v", a)
	}
}
