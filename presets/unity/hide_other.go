//go:build !windows

package unity

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
