package godot

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/ext"
)

func TestGodotPreset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "MyGame")
	for _, f := range []string{"project.godot", "Main.tscn", "player.gd", "player.gd.uid", "icon.svg", "icon.svg.import",
		".godot/imported/icon.svg-1.ctex", "export_presets.cfg", "export_credentials.cfg"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755)
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	os.WriteFile(filepath.Join(root, "project.godot"), []byte("[application]\n\nconfig/name=\"My Game\"\n"), 0o644)
	rules, err := ext.LoadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if a := rules.Applied(); len(a) != 1 || a[0].Preset != "godot" {
		t.Fatalf("applied: %+v", a)
	}
	for path, ignored := range map[string]bool{
		".godot": true, ".godot/imported/icon.svg-1.ctex": true, ".import": true, "export_credentials.cfg": true,
		"data_MyGame_windows_x86_64": true, "Game/fr.translation": true,
		"Main.tscn": false, "player.gd": false, "player.gd.uid": false, "icon.svg.import": false,
		"export_presets.cfg": false, "MyGame.csproj": false, "Tools/tool.exe": false, "Game.pck": false,
	} {
		dir := filepath.Ext(path) == "" || path == ".godot" || path == ".import"
		if got := rules.Ignored(path, dir); got != ignored {
			t.Errorf("%s: ignored = %v (%s)", path, got, rules.Explain(path, dir).By)
		}
	}
	for _, f := range []string{"Main.tscn", "items.tres", "player.gd", "project.godot", "icon.svg.import"} {
		if rules.Handler(f).Merge != "text" {
			t.Errorf("%s: merge %q", f, rules.Handler(f).Merge)
		}
	}
	if r := rules.Running(); len(r) != 1 || r[0] != "godot-editor" {
		t.Errorf("running: %v", r)
	}
	if n := projectName(root); n != "My Game" {
		t.Errorf("name %q", n)
	}
	for exe, want := range map[string]bool{"Godot_v4.3-stable_win64.exe": true, "godot.exe": true,
		"Godot_v4.3-stable_mono_win64_console.exe": true, "Unity.exe": false} {
		if isGodot(exe) != want {
			t.Errorf("%s: %v", exe, !want)
		}
	}
}
