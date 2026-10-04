package unreal

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/ext"
)

func TestUnrealPreset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "MyGame")
	for _, f := range []string{"MyGame.uproject", "Content/Maps/Main.umap", "Content/Hero.uasset", "Source/MyGame/Hero.cpp",
		"Config/DefaultEngine.ini", "Binaries/Win64/x.dll", "Saved/Logs/a.log", "Plugins/Fx/Binaries/Win64/y.dll"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755)
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	rules, err := ext.LoadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if a := rules.Applied(); len(a) != 1 || a[0].Preset != "unreal" {
		t.Fatalf("applied: %+v", a)
	}
	for path, ignored := range map[string]bool{
		"Binaries/Win64/x.dll": true, "Saved/Logs/a.log": true, "Intermediate": true, "DerivedDataCache": true,
		"Plugins/Fx/Binaries/Win64/y.dll": true, "Content/Hero.uasset": false, "Source/MyGame/Hero.cpp": false,
		"Content/Binaries/b.uasset": false,
	} {
		dir := filepath.Ext(path) == ""
		if got := rules.Ignored(path, dir); got != ignored {
			t.Errorf("%s: ignored = %v (%s)", path, got, rules.Explain(path, dir).By)
		}
	}
	if rules.Handler("Source/MyGame/Hero.cpp").Merge != "text" || rules.Handler("Config/DefaultEngine.ini").Merge != "text" ||
		rules.Handler("Content/Hero.uasset").Merge != "" {
		t.Error("merge handlers")
	}
	if rules.Kind("Content/Maps/Main.umap") != "level" || rules.Kind("Content/Hero.uasset") != "asset" {
		t.Error("kinds")
	}
}
