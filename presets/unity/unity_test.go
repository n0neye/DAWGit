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
		"Library": true, "Library/ArtifactDB": true, "Game.sln": true, "Temp": true, "UserSettings": true, ".utmp": true,
		"Assets/Player.cs": false, "Android/game.apk": true, "Assets/Models/a.blend1": true, "Assets/Tool/Tool.csproj": true,
		"Assets/StreamingAssets/aa/catalog.json": true, "Assets/StreamingAssets/movie.mp4": false, "Assets/InitTestScene12.unity": true,
		"Assets/Scenes/Main.unity": false, "Assets/Store/pack.unitypackage": false, "Logs/x.log": true, "Assets/Player.cs.meta": false, "Assets/Library/x.png": false,
	} {
		if got := rules.Ignored(path, filepath.Ext(path) == "" && path != "Library/ArtifactDB" || path == ".utmp"); got != ignored {
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
	os.MkdirAll(filepath.Join(root, "Assets", "Empty"), 0o755)
	c := func(p, status string) ext.Change { return ext.Change{Path: p, Status: status} }
	got := checkMeta(root, []ext.Change{
		c("Assets/ok.png", "added"), c("Assets/ok.png.meta", "added"),
		c("Assets/new.png", "added"), // no .meta yet
		c("Assets/orphan.wav.meta", "added"),
		c("Assets/kept.mat", "unchanged"), c("Assets/kept.mat.meta", "deleted"),
		c("Assets/gone.fbx", "deleted"), c("Assets/gone.fbx.meta", "unchanged"),
		c("Assets/old.png", "unchanged"),                                           // committed without one before
		c("Assets/Empty.meta", "added"),                                            // an empty folder's
		c("Assets/Art/a.png", "modified"), c("Assets/Art/a.png.meta", "unchanged"), // the folder has none
		// Not imported by Unity: no .meta needed.
		c("Assets/.sample.json", "added"), c("Assets/LICENSE~", "added"), c("Assets/x.tmp", "added"),
		c("Assets/Docs~/readme.md", "added"), c("Assets/.hidden/a.png", "added"), c("Assets/CVS/Entries", "added"),
		// A package's files have one; the package folder and Packages/*.json don't.
		c("Packages/manifest.json", "modified"),
		c("Packages/com.me.tool/package.json", "unchanged"), c("Packages/com.me.tool/package.json.meta", "unchanged"),
		c("Packages/com.me.tool/Samples~/s.cs", "added"),
		c("Packages/com.me.tool/Runtime.meta", "unchanged"), c("Packages/com.me.tool/Runtime/t.cs", "added"),
	})
	want := []string{
		"gone.fbx is deleted but its .meta",
		"kept.mat.meta is deleted",
		"new.png has no .meta yet",
		"old.png has no .meta: open",
		"orphan.wav.meta has no asset",
		"Runtime/t.cs has no .meta yet",
		"folder Assets/Art has no .meta",
	}
	if len(got) != len(want) {
		t.Fatalf("warnings:\n%s", strings.Join(got, "\n"))
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
