// Package livecheck detects a running Ableton Live, which would keep an old
// copy of a set in memory and overwrite files DAWGit changed on disk.
package livecheck

import "strings"

// Running reports whether an Ableton Live process appears to be running.
// Detection failures report false.
func Running() bool {
	names, err := processNames()
	if err != nil {
		return false
	}
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), "ableton live") {
			return true
		}
	}
	return false
}
