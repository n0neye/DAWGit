// Package handlers holds the built-in code a preset can name (see profile):
// merging a kind of file, and checking whether a tool has the project open.
// Extensions add their own through dawgit/ext.
package handlers

import (
	"sync"

	"dawgit/internal/livecheck"
	"dawgit/internal/textmerge"
)

// MergeFunc merges a file changed on both sides from its three versions.
// clean is false when the changes collide; the file is then chosen whole
// (yours, theirs or both), as for files without a handler.
type MergeFunc func(base, ours, theirs []byte) (merged []byte, clean bool, err error)

// RunningFunc tells what of the project in root a tool has open ("" when
// nothing; "?" when the tool runs but that can't be told): DAWGit doesn't
// rewrite files while it does.
type RunningFunc func(root string) string

// OpenFunc opens a project's file (or folder: rel ".") in its tool.
type OpenFunc func(root, rel string) error

// Change is a file changed since the version the project is on.
type Change struct {
	Path   string // relative, slash separated
	Status string // added | modified | deleted | untracked
}

// CheckFunc looks at what is about to be committed and returns warnings
// (shown before committing; the user may go on).
type CheckFunc func(root string, changes []Change) []string

var (
	mu       sync.RWMutex
	merges   = map[string]MergeFunc{}
	runnings = map[string]RunningFunc{}
	openers  = map[string]OpenFunc{}
	checks   = map[string]CheckFunc{}
)

// RegisterOpener adds an opener (the name a preset's open: with: uses).
func RegisterOpener(name string, fn OpenFunc) {
	mu.Lock()
	defer mu.Unlock()
	openers[name] = fn
}

// Opener returns an opener (nil when none has that name).
func Opener(name string) OpenFunc {
	mu.RLock()
	defer mu.RUnlock()
	return openers[name]
}

// RegisterCheck adds a pre-commit check (the name a preset's checks: lists).
func RegisterCheck(name string, fn CheckFunc) {
	mu.Lock()
	defer mu.Unlock()
	checks[name] = fn
}

// Check returns a pre-commit check (nil when none has that name).
func Check(name string) CheckFunc {
	mu.RLock()
	defer mu.RUnlock()
	return checks[name]
}

func init() {
	RegisterRunning("ableton-live", livecheck.OpenSet)
	RegisterMerge("text", textmerge.Merge)
}

// RegisterMerge adds a merge handler (the name a preset's merge: uses).
func RegisterMerge(name string, fn MergeFunc) {
	mu.Lock()
	defer mu.Unlock()
	merges[name] = fn
}

// Merge returns a merge handler (nil when none has that name).
func Merge(name string) MergeFunc {
	mu.RLock()
	defer mu.RUnlock()
	return merges[name]
}

// RegisterRunning adds a running-tool check (the name a preset's running:
// uses).
func RegisterRunning(name string, fn RunningFunc) {
	mu.Lock()
	defer mu.Unlock()
	runnings[name] = fn
}

// Running returns a running-tool check (nil when none has that name).
func Running(name string) RunningFunc {
	mu.RLock()
	defer mu.RUnlock()
	return runnings[name]
}
