// Package watch tells when files change under a folder.
package watch

import (
	"context"
	"errors"
	"time"
)

// ErrUnsupported: no watcher on this system (callers keep polling).
var ErrUnsupported = errors.New("watching folders is not supported here")

// Dir calls onChange with the changed paths (slash separated, relative to
// root) once things have been quiet for the given time, until ctx ends. An
// empty list means "something changed" (too many changes to list).
func Dir(ctx context.Context, root string, quiet time.Duration, onChange func(paths []string)) error {
	events := make(chan string, 256)
	if err := watch(ctx, root, events); err != nil {
		return err
	}
	go func() {
		var pending []string
		seen := map[string]bool{}
		overflow := false
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case p, ok := <-events:
				if !ok {
					return
				}
				if p == "" {
					overflow = true
				} else if !seen[p] {
					seen[p] = true
					pending = append(pending, p)
				}
				timer.Reset(quiet)
			case <-timer.C:
				if overflow {
					pending = nil
				}
				onChange(pending)
				pending, seen, overflow = nil, map[string]bool{}, false
			}
		}
	}()
	return nil
}
