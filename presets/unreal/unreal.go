// Package unreal teaches DAWGit about Unreal Engine projects: what to leave
// out (Binaries, Intermediate, Saved, DerivedDataCache), C++ and config
// merged line by line, and not rewriting files while the Unreal Editor has
// the project open. Assets (.uasset, .umap) are binary: when two people
// change the same one, you choose whose to keep. Importing it is enough.
package unreal

import (
	_ "embed"
	"path/filepath"
	"strings"

	"dawgit/ext"
	"dawgit/internal/wintitle"
)

//go:embed unreal.yaml
var preset []byte

func init() {
	if err := ext.RegisterPreset(preset); err != nil {
		panic("unreal preset: " + err.Error())
	}
	ext.RegisterRunning("unreal-editor", editorOpen)
}

// editorOpen reports "Unreal Editor" when an editor window has this project
// open; its title names the project: "MyGame - Unreal Editor".
func editorOpen(root string) string {
	projects, _ := filepath.Glob(filepath.Join(root, "*.uproject"))
	titles := wintitle.Of("UnrealEditor.exe")
	for _, p := range projects {
		name := strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		for _, t := range titles {
			if t = strings.ToLower(t); strings.Contains(t, name) && strings.Contains(t, "unreal editor") {
				return "Unreal Editor"
			}
		}
	}
	return ""
}
