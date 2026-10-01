package project

import (
	"errors"
	"path/filepath"
	"time"
)

// ErrBusy: another DAWGit (the app, the command line, another edition) is
// working on the project.
var ErrBusy = errors.New("another DAWGit window or command is working on this project; try again when it's done")

// Lock keeps other programs from changing the project until the returned
// function is called (or this program ends), waiting up to wait for one
// that has it. Within one program, callers take turns themselves.
func (r *Repo) Lock(wait time.Duration) (func(), error) {
	path := filepath.Join(r.Dir, "lock")
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
		time.Sleep(100 * time.Millisecond)
	}
}

var errLocked = errors.New("locked")
