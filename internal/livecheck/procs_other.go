//go:build !windows

package livecheck

import (
	"os/exec"
	"strings"
)

func processNames() ([]string, error) {
	out, err := exec.Command("ps", "-axo", "comm").Output()
	if err != nil {
		return nil, err
	}
	return strings.Split(string(out), "\n"), nil
}
