package design

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/ext"
	_ "dawgit/presets/code"
	_ "dawgit/presets/unity"
)

func write(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("x"), 0o644)
}

func TestDesignPreset(t *testing.T) {
	root := t.TempDir()
	write(t, root, "poster.psd")
	write(t, root, ".gitignore") // design before code
	rules, err := ext.LoadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if a := rules.Applied(); len(a) != 1 || a[0].Preset != "design" {
		t.Fatalf("applied: %+v", a)
	}
	for rel, ignored := range map[string]bool{
		"scene.blend": false, "scene.blend1": true, "scene.blend12": true, "scene.blend@": true,
		"rig.mb": false, "rig.mb.swatches": true, "incrementalSave/rig.0001.mb": true,
		"city.c4d": false, "backup/city_backup_3.c4d": true,
		"CacheClip/a.dvcc": true, "Footage/ProxyMedia/a.mov": true, "Footage/a.mov": false,
	} {
		if got := rules.Ignored(rel, false); got != ignored {
			t.Errorf("%s: ignored = %v", rel, got)
		}
	}
	for rel, kind := range map[string]string{"poster.psd": "design", "a.blend": "model", "show.avc": "scene",
		"clip.mov": "video", "tex.exr": "image"} {
		if got := rules.Kind(rel); got != kind {
			t.Errorf("%s: kind %q", rel, got)
		}
	}
	// Files one folder down are enough.
	sub := t.TempDir()
	write(t, sub, "Art/scene.blend")
	if a, _ := ext.LoadRules(sub); len(a.Applied()) != 1 || a.Applied()[0].Preset != "design" {
		t.Fatalf("one folder down: %+v", a.Applied())
	}
	// A Unity project with .psd files in Assets is a Unity project.
	unity := t.TempDir()
	write(t, unity, "ProjectSettings/ProjectVersion.txt")
	write(t, unity, "Assets/hero.psd")
	if a, _ := ext.LoadRules(unity); a.Applied()[0].Preset != "unity" {
		t.Fatalf("unity: %+v", a.Applied())
	}
}

func TestProjectNames(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Art/Hero.PSD")
	write(t, root, "Art/notes.txt")
	write(t, root, ".dawgit/objects/x.psd")
	got := projectNames(root)
	if len(got) != 1 || got[0] != "hero.psd" {
		t.Fatalf("names: %v", got)
	}
}
