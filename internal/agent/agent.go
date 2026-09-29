// Package agent watches a project while its owner works in Live: it backs up
// unsaved sets to the server, tracks what teammates are editing (soft locks)
// and notices new versions. It never changes project files.
package agent

import (
	"context"
	"sort"
	"time"

	"dawgit/internal/project"
)

type EventKind string

const (
	BackedUp        EventKind = "backed-up"        // Labels: your unsaved edits
	NewVersions     EventKind = "new-versions"     // Versions: saved by the team
	TeammateEditing EventKind = "teammate-editing" // Author started editing Labels
	TeammateIdle    EventKind = "teammate-idle"    // Author has no unsaved work any more
	Overlap         EventKind = "overlap"          // Text: you and someone edit the same track
	Offline         EventKind = "offline"          // Text: error
	Online          EventKind = "online"
)

type Event struct {
	Kind     EventKind
	Author   string
	Labels   []string
	Versions []*project.Manifest
	Text     string
	// Waiting marks versions that were already there when the agent started.
	Waiting bool
}

// Label formats an edit as "Song.als: Bass".
func Label(e project.TrackEdit) string { return e.Set + ": " + e.Name }

// Watcher holds the agent's memory between checks.
type Watcher struct {
	Root string

	lastSig     string
	lastReport  time.Time
	myEdits     []project.TrackEdit
	notified    map[string]bool
	mateEditing map[string]map[string]bool
	warned      map[string]bool
	offline     bool
	started     bool
}

func New(root string) *Watcher {
	return &Watcher{Root: root, notified: map[string]bool{}, mateEditing: map[string]map[string]bool{},
		warned: map[string]bool{}}
}

// Check runs one round and returns what happened.
func (w *Watcher) Check() []Event {
	var events []Event
	// Re-open each time: `switch` may have changed the branch.
	r, err := project.Open(w.Root)
	if err != nil {
		return []Event{{Kind: Offline, Text: err.Error()}}
	}

	// 1. Report unsaved work when a set was saved in Live (or hourly).
	if sig := r.SetsSignature(); sig != w.lastSig || time.Since(w.lastReport) > time.Hour {
		st, err := r.ReportWorkspace()
		if err == nil {
			w.lastSig, w.lastReport, w.myEdits = sig, time.Now(), st.Edits
			if len(st.Edits) > 0 {
				var labels []string
				for _, e := range st.Edits {
					labels = append(labels, Label(e))
				}
				events = append(events, Event{Kind: BackedUp, Labels: labels})
			}
		}
		switch {
		case err != nil && !w.offline:
			w.offline = true
			events = append(events, Event{Kind: Offline, Text: err.Error()})
		case err == nil && w.offline:
			w.offline = false
			events = append(events, Event{Kind: Online})
		}
		if err != nil {
			return events
		}
	}

	// 2. New versions from the team.
	if incoming, err := r.IncomingVersions(); err == nil {
		var fresh []*project.Manifest
		for _, m := range incoming {
			if !w.notified[m.ID] {
				w.notified[m.ID] = true
				fresh = append(fresh, m)
			}
		}
		if len(fresh) > 0 {
			events = append(events, Event{Kind: NewVersions, Versions: fresh, Waiting: !w.started})
		}
	}
	w.started = true

	// 3. What teammates are editing, and overlaps with you.
	if mates, err := r.Teammates(); err == nil {
		seen := map[string]bool{}
		for _, m := range mates {
			seen[m.Author] = true
			now := map[string]bool{}
			for _, e := range m.Edits {
				now[Label(e)] = true
			}
			var started []string
			for l := range now {
				if !w.mateEditing[m.Author][l] {
					started = append(started, l)
				}
			}
			sort.Strings(started)
			if len(started) > 0 {
				events = append(events, Event{Kind: TeammateEditing, Author: m.Author, Labels: started})
			}
			w.mateEditing[m.Author] = now
		}
		var gone []string
		for author := range w.mateEditing {
			if !seen[author] {
				gone = append(gone, author)
			}
		}
		sort.Strings(gone)
		for _, author := range gone {
			delete(w.mateEditing, author)
			events = append(events, Event{Kind: TeammateIdle, Author: author})
		}
		for _, o := range project.Overlaps(w.myEdits, mates) {
			if !w.warned[o] {
				w.warned[o] = true
				events = append(events, Event{Kind: Overlap, Text: o})
			}
		}
	}
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
