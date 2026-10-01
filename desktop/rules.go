package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/profile"
	"dawgit/internal/version"
)

// rulesTemplate starts a project's .dawgit.yaml: what it is, and the two
// rules people ask for most, commented out.
const rulesTemplate = `# DAWGit's rules for this project: which files are left out of versions.
# This file is committed with the project, so the whole team uses the same rules.
# Guide: https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md
requires: "%s"
use:
  ./: ableton
rules:
  # Later rules win. Ignored files stay on everyone's disk.
  # - ignore: "Exports/"    # leave a folder out of versions
  # - track: "*.asd"        # keep Live's analysis files after all
`

// OpenRules opens the project's .dawgit.yaml in a text editor, creating it
// from a commented template first.
func (a *App) OpenRules(root string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	p := filepath.Join(root, profile.FileName)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		// This DAWGit's version (major.minor) is the oldest that follows it.
		v := version.Version
		if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
			v = parts[0] + "." + parts[1]
		}
		if err := os.WriteFile(p, []byte(fmt.Sprintf(rulesTemplate, v)), 0o644); err != nil {
			return err
		}
	}
	return shellEdit(p)
}
