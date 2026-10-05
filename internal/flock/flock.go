// Package flock locks a file between processes (and goroutines): the app,
// the command line tool and background work can all reach the same project
// or settings, and only one may change them at a time.
package flock

import (
	"errors"
	"time"
)

// ErrBusy: the lock stayed taken for as long as the caller would wait.
var ErrBusy = errors.New("locked by another DAWGit")

var errLocked = errors.New("locked")

// Lock takes the lock on path (the file is made if needed), waiting up to
// wait for it; the returned func releases it. The lock also ends with the
// process.
func Lock(path string, wait time.Duration) (func(), error) {
	deadline := time.Now().Add(wait)
	for {
		unlock, err := lockFile(path)
		if err == nil {
			return unlock, nil
		}
		if !errors.Is(err, errLocked) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, ErrBusy
		}
		time.Sleep(20 * time.Millisecond)
	}
}
