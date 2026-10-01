package desktop

import (
	"log"
	"sync"

	"dawgit/internal/project"
)

// tidying: projects with a tidy-up queued (one at a time per project).
var tidying = struct {
	sync.Mutex
	pending map[string]bool
	running sync.WaitGroup
}{pending: map[string]bool{}}

// waitTidy waits for queued tidy-ups (tests, before their folders go).
func waitTidy() { tidying.running.Wait() }

// tidyLater tidies a project's .dawgit in the background once the current
// operation lets go of it: files the team's storage has are removed here
// (team projects), and objects no version refers to are deleted.
func (a *App) tidyLater(root string) {
	tidying.Lock()
	if tidying.pending[root] {
		tidying.Unlock()
		return
	}
	tidying.pending[root] = true
	tidying.running.Add(1)
	tidying.Unlock()
	go func() {
		defer tidying.running.Done()
		unlock := a.lock(root)
		defer unlock()
		tidying.Lock()
		delete(tidying.pending, root)
		tidying.Unlock()
		r, err := project.Open(root)
		if err != nil {
			return
		}
		pruned, err := r.PruneObjects()
		if err != nil && tracing {
			log.Printf("trace tidy %s: %v", root, err)
		}
		freed, err := r.GC()
		if err != nil && tracing {
			log.Printf("trace tidy %s: gc: %v", root, err)
		}
		if tracing {
			log.Printf("trace tidy %s: %d MB moved to the team's storage, %d MB garbage", root, pruned>>20, freed>>20)
		}
	}()
}
