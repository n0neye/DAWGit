// Package godot teaches DAWGit about Godot projects: what to leave out
// (.godot/ and other caches), scenes, resources and scripts merged line by
// line (Godot saves them as text), and not rewriting files while the Godot
// editor has the project open. Importing it is enough.
package godot

import (
	_ "embed"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"dawgit/ext"
	"dawgit/internal/wintitle"
)

//go:embed godot.yaml
var preset []byte

func init() {
	if err := ext.RegisterPreset(preset); err != nil {
		panic("godot preset: " + err.Error())
	}
	ext.RegisterRunning("godot-editor", editorOpen)
}

var nameLine = regexp.MustCompile(`(?m)^config/name="(.*)"\s*$`)

// projectName is the name in project.godot ("" when it has none).
func projectName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		return ""
	}
	if m := nameLine.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}

// isGodot: Godot's file name has its version in it
// (Godot_v4.3-stable_win64.exe), or not (godot.exe).
func isGodot(exe string) bool {
	exe = strings.ToLower(exe)
	return strings.HasPrefix(exe, "godot") && strings.HasSuffix(exe, ".exe")
}

// editorOpen reports "Godot" when an editor window has this project open;
// its title names the project: "My Game - Main.tscn - Godot Engine".
func editorOpen(root string) string {
	name := strings.ToLower(projectName(root))
	if name == "" {
		return ""
	}
	for _, t := range wintitle.Matching(isGodot) {
		if t = strings.ToLower(t); strings.Contains(t, name) && strings.Contains(t, "godot engine") {
			return "Godot"
		}
	}
	return ""
}
