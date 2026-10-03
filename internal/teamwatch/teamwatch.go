// Package teamwatch watches a project while its owner works in Live: it
// notices versions the team committed on the project's branch. It never
// changes project files.
package teamwatch

import (
	"context"
	"time"

	"dawgit/internal/project"
)

type EventKind string

const (
	NewVersions EventKind = "new-versions" // Versions: committed by the team
	Offline     EventKind = "offline"      // Text: error
	Online      EventKind = "online"
)

type Event struct {
	Kind     EventKind
	Author   string
	Labels   []string
	Versions []*project.Manifest
	Text     string
	// Waiting: the versions were already there when the watch started.
	Waiting bool
}

// Label is "Song.als: Bass" (or a set-wide change like "tempo: 120 -> 128").
func Label(e project.TrackEdit) string { return e.Set + ": " + e.Name }

// Watcher remembers what it already reported between checks.
type Watcher struct {
	Root string

	notified map[string]bool
	offline  bool
	started  bool
}

func New(root string) *Watcher {
	return &Watcher{Root: root, notified: map[string]bool{}}
}

// Check runs one round and returns what happened. It reads the branch's head
// only (one small request) and needs no project lock.
func (w *Watcher) Check() []Event {
	var events []Event
	// Re-open each time: `switch` may have changed the branch.
	r, err := project.Open(w.Root)
	if err != nil {
		return []Event{{Kind: Offline, Text: err.Error()}}
	}
	incoming, err := r.IncomingVersions()
	switch {
	case err != nil && !w.offline:
		w.offline = true
		return append(events, Event{Kind: Offline, Text: err.Error()})
	case err != nil:
		return events
	case w.offline:
		w.offline = false
		events = append(events, Event{Kind: Online})
	}
	var fresh []*project.Manifest
	for _, m := range incoming {
		if !w.notified[m.ID] {
			w.notified[m.ID] = true
			// A merge only combines versions the team is told about anyway.
			if len(m.Parents) < 2 {
				fresh = append(fresh, m)
			}
		}
	}
	if len(fresh) > 0 {
		events = append(events, Event{Kind: NewVersions, Versions: fresh, Waiting: !w.started})
	}
	w.started = true
	return events
}

// Run checks every interval until ctx is done, passing events to emit.
func Run(ctx context.Context, root string, interval time.Duration, emit func(Event)) {
	w := New(root)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		for _, e := range w.Check() {
			emit(e)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
