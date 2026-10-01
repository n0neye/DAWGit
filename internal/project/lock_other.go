//go:build !windows

package project

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on path, released when it is closed or
// this program ends.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, errLocked
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}
