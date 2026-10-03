//go:build !windows

package cli

import "errors"

func userPath(add bool, dir string) (bool, error) {
	return false, errors.New("dawgit path is for Windows; elsewhere, add the folder to PATH in your shell's profile")
}
