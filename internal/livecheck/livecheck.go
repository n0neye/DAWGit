// Package livecheck detects a set open in Ableton Live, which keeps its own
// copy in memory and would overwrite files DAWGit changed on disk.
package livecheck

import (
	"errors"
	"path/filepath"
	"strings"
)

var errUnsupported = errors.New("not supported on this system")

func isLive(exe string) bool { return strings.Contains(strings.ToLower(exe), "ableton live") }

// Running reports whether an Ableton Live process appears to be running.
// Detection failures report false.
func Running() bool {
	names, err := processNames()
	if err != nil {
		return false
	}
	for _, n := range names {
		if isLive(n) {
			return true
		}
	}
	return false
}

// SetName extracts the set's name from a Live window title such as
// "Song - Ableton Live 12 Suite", "Song* - Ableton Live 12 Suite" (unsaved)
// or "Song [Song Project] - Ableton Live 11 Suite"; "" for other windows.
func SetName(title string) string {
	i := strings.LastIndex(title, " - Ableton Live")
	if i <= 0 {
		return ""
	}
	name := strings.TrimSpace(title[:i])
	if j := strings.LastIndex(name, " ["); j > 0 && strings.HasSuffix(name, "]") {
		name = name[:j]
	}
	return strings.TrimSpace(strings.TrimSuffix(name, "*"))
}

// OpenSet returns the name of a set of the project in root that is open in
// Live, or "". Live's window title only shows the set's name, so a set of
// another project with the same name counts too. When Live runs but the
// open set cannot be told, it answers "?" (better to ask once too often).
func OpenSet(root string) string {
	if !Running() {
		return ""
	}
	titles, err := liveWindowTitles()
	if err != nil {
		return "?"
	}
	sets, _ := filepath.Glob(filepath.Join(root, "*.als"))
	for _, t := range titles {
		name := SetName(t)
		if name == "" {
			continue
		}
		for _, s := range sets {
			if strings.EqualFold(strings.TrimSuffix(filepath.Base(s), filepath.Ext(s)), name) {
				return filepath.Base(s)
			}
		}
	}
	return ""
}
