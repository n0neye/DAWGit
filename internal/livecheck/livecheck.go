// Package livecheck detects a running Ableton Live, which would keep an old
// copy of a set in memory and overwrite files DAWGit changed on disk.
package livecheck

import (
	"os/exec"
	"runtime"
	"strings"
)

// Running reports whether an Ableton Live process appears to be running.
// Detection failures report false.
func Running() bool {
	var out []byte
	var err error
	switch runtime.GOOS {
	case "windows":
		out, err = exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	case "darwin", "linux":
		out, err = exec.Command("ps", "-axo", "comm").Output()
	default:
		return false
	}
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "ableton live")
}
