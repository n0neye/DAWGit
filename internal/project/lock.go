package project

import (
	"errors"
	"path/filepath"
	"time"

	"dawgit/internal/flock"
)

// ErrBusy: another DAWGit (the app, the command line, another edition) is
// working on the project.
var ErrBusy = errors.New("another DAWGit window or command is working on this project; try again when it's done")

// Lock keeps other programs from changing the project until the returned
// function is called (or this program ends), waiting up to wait for one
// that has it. Within one program, callers take turns themselves.
func (r *Repo) Lock(wait time.Duration) (func(), error) {
	unlock, err := flock.Lock(filepath.Join(r.Dir, "lock"), wait)
	if errors.Is(err, flock.ErrBusy) {
		return nil, ErrBusy
	}
	return unlock, err
}
