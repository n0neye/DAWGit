// Package code teaches DAWGit about software projects: the project's
// .gitignore files are followed, the usual tool folders (node_modules,
// virtual environments, caches) and .env secrets are left out, and source
// files are merged line by line. Its priority is low: a folder that is
// also a Unity, Unreal or Live project is that. Importing it is enough.
package code

import (
	_ "embed"

	"dawgit/ext"
)

//go:embed code.yaml
var preset []byte

func init() {
	if err := ext.RegisterPreset(preset); err != nil {
		panic("code preset: " + err.Error())
	}
}
