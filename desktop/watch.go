package desktop

import (
	"context"
	"path"
	"slices"
	"strings"
	"time"

	"dawgit/internal/profile"
	"dawgit/internal/watch"
)

// folderWatch watches an open project's folder for the views showing it.
type folderWatch struct {
	users  int
	cancel context.CancelFunc
}

// FilesEvent: files changed in a project folder (outside DAWGit).
type FilesEvent struct {
	Root string `json:"root"`
}

// WatchFiles sends "files" events when files in the project change, so new
// or edited samples show up without waiting for the next poll. Sets are left
// to Signature (it waits for Live to finish writing). It returns false when
// this system has no watcher; each call wants an UnwatchFiles.
func (a *App) WatchFiles(root string) bool {
	if !knownProject(root) {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if w, ok := a.watches[root]; ok {
		w.users++
		return true
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := watch.Dir(ctx, root, 500*time.Millisecond, func(paths []string) {
		if len(paths) > 0 {
			rules, _ := profile.Load(root)
			if !slices.ContainsFunc(paths, func(p string) bool { return worthReloading(rules, p) }) {
				return
			}
		}
		if a.emit != nil {
			a.emit("files", FilesEvent{Root: root})
		}
	})
	if err != nil {
		cancel()
		return false
	}
	a.watches[root] = &folderWatch{users: 1, cancel: cancel}
	return true
}

// UnwatchFiles ends a WatchFiles.
func (a *App) UnwatchFiles(root string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if w, ok := a.watches[root]; ok {
		if w.users--; w.users <= 0 {
			w.cancel()
			delete(a.watches, root)
		}
	}
}

// worthReloading: a change DAWGit shows (not one the project's rules leave
// out, a set in the root or a conversion still being written).
func worthReloading(rules *profile.Profile, rel string) bool {
	if rules.Ignored(rel, false) || strings.Contains(path.Base(rel), ".part.") {
		return false
	}
	return !(strings.EqualFold(path.Ext(rel), ".als") && !strings.Contains(rel, "/"))
}
