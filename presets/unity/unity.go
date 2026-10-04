// Package unity teaches DAWGit about Unity projects: what to leave out
// (Library/ and other caches), code merged line by line, scenes and
// prefabs merged with Unity's UnityYAMLMerge, not rewriting files
// while the Unity Editor has the project open, opening the project in its
// Unity version, and warning when an asset and its .meta part ways.
// Importing it is enough.
package unity

import (
	_ "embed"

	"dawgit/ext"
)

//go:embed unity.yaml
var preset []byte

func init() {
	if err := ext.RegisterPreset(preset); err != nil {
		panic("unity preset: " + err.Error())
	}
	ext.RegisterRunning("unity-editor", editorOpen)
	ext.RegisterOpener("unity-editor", openEditor)
	ext.RegisterCheck("unity-meta", checkMeta)
	ext.RegisterMerge("unity-yaml", mergeYAML)
}
