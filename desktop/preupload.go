package desktop

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"dawgit/internal/project"
	"dawgit/internal/teams"
)

// Big files go up to the team's storage in the background once they stop
// changing (project.Preupload): one at a time, never while the project is
// busy, and without holding it up (only recording one takes its lock).

const preuploadCheck = time.Minute

// Preupload is a file going up in the background (event "preupload": Done
// false while it goes, true once it's up or stopped).
type Preupload struct {
	Root  string `json:"root"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Total int64  `json:"total"`
	Done  bool   `json:"done"`
}

var (
	preuploadMu  sync.Mutex
	preuploading = map[string]Preupload{} // root
)

// Preuploads lists the files going up in the background now.
func (a *App) Preuploads() []Preupload {
	preuploadMu.Lock()
	defer preuploadMu.Unlock()
	out := []Preupload{}
	for _, p := range preuploading {
		out = append(out, p)
	}
	return out
}

// SetPreupload turns the background upload of big files on or off for
// team teamID on this computer.
func (a *App) SetPreupload(teamID string, on bool) error {
	_, err := updateTeam(teamID, func(_ *teams.Store, t *teams.Team) error {
		t.NoPreupload = !on
		return nil
	})
	return err
}

func (a *App) notePreupload(p Preupload) {
	preuploadMu.Lock()
	if p.Done {
		delete(preuploading, p.Root)
	} else {
		preuploading[p.Root] = p
	}
	preuploadMu.Unlock()
	if a.emit != nil {
		a.emit("preupload", p)
	}
}

// busy: an action holds the project now.
func (a *App) busy(root string) bool {
	a.mu.Lock()
	l, ok := a.locks[root]
	a.mu.Unlock()
	if !ok {
		return false
	}
	if l.TryLock() {
		l.Unlock()
		return false
	}
	return true
}

// preuploadOnSchedule looks for big files to put up early, every minute
// while the app runs.
func (a *App) preuploadOnSchedule(ctx context.Context) {
	if store, err := teams.Load(); err == nil {
		for _, root := range store.Roots() {
			if r, err := project.Open(root); err == nil {
				r.CleanPreuploads() // left by an upload the app closed on
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(preuploadCheck):
		}
		store, err := teams.Load()
		if err != nil {
			continue
		}
		for _, t := range store.Teams {
			if t.NoPreupload || !t.Remote.IsStorage() {
				continue
			}
			for key, root := range store.Projects {
				if strings.HasPrefix(key, t.ID+"/") && ctx.Err() == nil {
					a.preuploadProject(ctx, root)
				}
			}
		}
	}
}

func (a *App) preuploadProject(ctx context.Context, root string) {
	if a.busy(root) {
		return
	}
	r, err := project.Open(root)
	if err != nil || r.Config.Remote == nil {
		return
	}
	cands, err := r.PreuploadCandidates(time.Now())
	if err != nil || len(cands) == 0 {
		return
	}
	c, err := r.Client()
	if err != nil {
		return
	}
	for _, cand := range cands {
		if ctx.Err() != nil || a.busy(root) {
			return
		}
		p := Preupload{Root: root, Path: cand.Path, Total: cand.Size}
		a.notePreupload(p)
		var last time.Time
		err := r.Preupload(c, cand, func(done, total int64) {
			if time.Since(last) > 500*time.Millisecond {
				last = time.Now()
				p.Bytes, p.Total = done, total
				a.notePreupload(p)
			}
		})
		if err == nil {
			err = a.recordPreupload(root, cand.Hash)
		}
		p.Done = true
		a.notePreupload(p)
		if err != nil && !errors.Is(err, project.ErrChangedSince) {
			log.Printf("preupload %s %s: %v", root, cand.Path, err)
			return // tried again next time
		}
	}
}

// recordPreupload notes the upload, with the project locked.
func (a *App) recordPreupload(root, hash string) error {
	r, unlock, err := a.open(root)
	if err != nil {
		return err
	}
	defer unlock()
	return r.NotePreuploaded(hash)
}
