package store

import (
	"errors"
	"io/fs"
	"os"
	"time"
)

// On Windows replacing or removing a file fails while it is read-only (as
// projects that came from Perforce are), or while another program holds it
// for a moment (virus scanners and search indexers open new files). Replace
// and Remove clear the read-only mark and try again for a couple of seconds.

// retryFor bounds the retries of one Replace or Remove.
var retryFor = 2 * time.Second

// Replace moves src over dst.
func Replace(src, dst string) error {
	return retry(dst, func() error { return os.Rename(src, dst) })
}

// Remove removes a file; a file that isn't there is an fs.ErrNotExist error.
func Remove(path string) error {
	return retry(path, func() error { return os.Remove(path) })
}

func retry(path string, op func() error) error {
	wait := 20 * time.Millisecond
	deadline := time.Now().Add(retryFor)
	for {
		err := op()
		if err == nil || errors.Is(err, fs.ErrNotExist) || time.Now().After(deadline) {
			return err
		}
		if fi, serr := os.Lstat(path); serr == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o200 == 0 {
			if os.Chmod(path, fi.Mode().Perm()|0o200) == nil {
				continue // read-only: cleared, try again now
			}
		}
		time.Sleep(wait)
		wait = min(wait*2, 400*time.Millisecond)
	}
}
