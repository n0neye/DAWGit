package unity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/ext"
)

func TestUnityPreset(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Game")
	for _, f := range []string{"ProjectSettings/ProjectVersion.txt", "Assets/Player.cs", "Assets/Player.cs.meta",
		"Assets/Main.unity", "Library/ArtifactDB", "Game.sln"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o755)
		os.WriteFile(filepath.Join(root, f), []byte("x"), 0o644)
	}
	rules, err := ext.LoadRules(root)
	if err != nil {
		t.Fatal(err)
	}
	if a := rules.Applied(); len(a) != 1 || a[0].Preset != "unity" || !a[0].Detected {
		t.Fatalf("applied: %+v", a)
	}
	for path, ignored := range map[string]bool{
		"Library": true, "Library/ArtifactDB": true, "Game.sln": true, "Temp": true, "UserSettings": true,
		"Assets/Player.cs": false, "Assets/Player.cs.meta": false, "Assets/Library/x.png": false,
	} {
		if got := rules.Ignored(path, filepath.Ext(path) == "" && path != "Library/ArtifactDB"); got != ignored {
			t.Errorf("%s: ignored = %v", path, got)
		}
	}
	if rules.Handler("Assets/Player.cs").Merge != "text" || rules.Handler("Assets/Main.unity").Merge != "unity-yaml" {
		t.Error("merge handlers")
	}
	if rules.Kind("Assets/Main.unity") != "scene" || rules.Kind("Assets/Player.cs") != "script" {
		t.Error("kinds")
	}
	if r := rules.Running(); len(r) != 1 || r[0] != "unity-editor" {
		t.Errorf("running: %v", r)
	}
}

func TestMetaCheck(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755)
		os.WriteFile(filepath.Join(root, rel), []byte("x"), 0o644)
	}
	write("Assets/ok.png")
	write("Assets/ok.png.meta")
	write("Assets/new.png") // no .meta yet
	write("Assets/orphan.wav.meta")
	write("Assets/kept.mat") // its .meta was deleted
	write("Assets/gone.fbx.meta")
	got := checkMeta(root, []ext.Change{
		{Path: "Assets/ok.png", Status: "added"}, {Path: "Assets/ok.png.meta", Status: "added"},
		{Path: "Assets/new.png", Status: "added"},
		{Path: "Assets/orphan.wav.meta", Status: "added"},
		{Path: "Assets/kept.mat.meta", Status: "deleted"},
		{Path: "Assets/gone.fbx", Status: "deleted"},
		{Path: "Packages/manifest.json", Status: "modified"},
	})
	want := []string{"new.png has no .meta", "orphan.wav.meta has no asset", "kept.mat.meta is deleted", "gone.fbx is deleted but its .meta"}
	if len(got) != len(want) {
		t.Fatalf("warnings: %q", got)
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("warning %d: %q, want %q", i, got[i], w)
		}
	}
}

func TestEditorVersionAndOpen(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "ProjectSettings"), 0o755)
	os.WriteFile(filepath.Join(root, "ProjectSettings", "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 1.2.3f4\nm_EditorVersionWithRevision: 1.2.3f4 (abc)\n"), 0o644)
	if v, err := editorVersion(root); err != nil || v != "1.2.3f4" {
		t.Fatalf("version: %q %v", v, err)
	}
	if err := openEditor(root, "."); err == nil || !strings.Contains(err.Error(), "Unity 1.2.3f4 isn't installed") {
		t.Fatalf("missing editor: %v", err)
	}
	rules, _ := ext.LoadRules(root)
	paths, openers := rules.Openable(root)
	if rules.Tool() != "Unity" || len(paths) != 1 || paths[0] != "." || openers["."] != "unity-editor" {
		t.Fatalf("open: %q %v %v", rules.Tool(), paths, openers)
	}
	if c := rules.Checks(); len(c) != 1 || c[0] != "unity-meta" {
		t.Fatalf("checks: %v", c)
	}
}
