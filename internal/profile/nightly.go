package profile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Project kinds still in testing: their presets come with the Nightly
// channel's builds (dawgit/presets). A Stable build says so instead of not
// knowing them.
var nightlyPresets = map[string]string{"unity": "Unity", "unreal": "Unreal", "godot": "Godot", "code": "Code", "design": "Design"}

// NeedsNightly is the error for a project kind only Nightly builds know.
type NeedsNightly struct{ Kind string }

func (e *NeedsNightly) Error() string {
	return fmt.Sprintf("%s projects are still in testing: they need DAWGit's Nightly channel (Settings → Updates)", e.Kind)
}

// NightlyKind names the kind of project in root that only Nightly builds
// know ("Unity", "Unreal", "Godot"), when this build doesn't ("" otherwise).
func NightlyKind(root string) string {
	has := func(name string) bool {
		_, ok := builtin[name]
		return ok
	}
	isDir := func(p string) bool {
		fi, err := os.Stat(filepath.Join(root, p))
		return err == nil && fi.IsDir()
	}
	if !has("unity") && isDir("Assets") && isDir("ProjectSettings") {
		return "Unity"
	}
	if m, _ := filepath.Glob(filepath.Join(root, "*.uproject")); len(m) > 0 && !has("unreal") {
		return "Unreal"
	}
	if _, err := os.Stat(filepath.Join(root, "project.godot")); err == nil && !has("godot") {
		return "Godot"
	}
	return ""
}
