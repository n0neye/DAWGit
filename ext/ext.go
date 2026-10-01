// Package ext is how code outside this repository extends DAWGit: more
// presets (tools DAWGit understands), merge handlers, running-tool checks
// and backends (where a team's versions live).
//
// An extension registers what it adds before the app starts, then runs the
// app as usual:
//
//	func main() {
//		ext.RegisterPreset(unityPreset)
//		ext.RegisterRunning("unity-editor", unityOpen)
//		desktop.Run()
//	}
//
// Everything else in this repository is internal and may change; this
// package is what stays put. See docs/extending.md.
package ext

import (
	"dawgit/internal/handlers"
	"dawgit/internal/profile"
	"dawgit/internal/remote"
	"dawgit/internal/version"
)

// SetEdition names this build (e.g. "Pro"): the app shows it next to its
// name and version, and doesn't offer the public app's updates (they would
// replace this build). Call it before the app starts.
func SetEdition(name string) { version.Edition = name }

// --- presets and handlers ---

// RegisterPreset adds a preset, in the YAML format of the built-in ones (see
// docs/profiles.md); a preset with the same name replaces it.
func RegisterPreset(yaml []byte) error { return profile.RegisterPreset(yaml) }

// Rules are a project's resolved rules: which preset applies where, and for
// each file whether it is tracked, its kind and handler (e.g. to test a
// preset).
type Rules = profile.Profile

// LoadRules reads a project folder's rules (its .dawgit.yaml, or detected).
func LoadRules(root string) (*Rules, error) { return profile.Load(root) }

// MergeFunc merges a file changed on both sides from its three versions;
// clean is false when the changes collide (the file is then chosen whole).
type MergeFunc = handlers.MergeFunc

// RegisterMerge adds a merge handler, named by a preset's merge: field.
func RegisterMerge(name string, fn MergeFunc) { handlers.RegisterMerge(name, fn) }

// RunningFunc tells what of a project a tool has open ("" when nothing, "?"
// when it can't be told); DAWGit doesn't rewrite files meanwhile.
type RunningFunc = handlers.RunningFunc

// RegisterRunning adds a running-tool check, named by a preset's running:
// field.
func RegisterRunning(name string, fn RunningFunc) { handlers.RegisterRunning(name, fn) }

// --- backends ---

type (
	// Backend is where a team's versions live. Built in: a team server and
	// S3-compatible storage.
	Backend = remote.Backend
	// Config is a team's address and credentials.
	Config = remote.Config
	// Project is a song or project in a team.
	Project = remote.Project
	// TeamInfo is what a team says about itself (its name).
	TeamInfo = remote.TeamInfo
	// Member is a person in the team.
	Member = remote.Member
	// ErrConflict is UpdateBranch's error when the branch has moved.
	ErrConflict = remote.ErrConflict
	// Capabilities are features beyond versions (locks, presence); a
	// backend offers them by implementing Capable.
	Capabilities = remote.Capabilities
	// Capable is implemented by backends with capabilities.
	Capable = remote.Capable
)

// ErrNotFound is what a backend returns for something it doesn't have.
var ErrNotFound = remote.ErrNotFound

// RegisterBackend adds a kind of backend: team addresses starting with
// prefix (e.g. "rtdb+https://") are opened by open.
func RegisterBackend(prefix string, open func(Config) (Backend, error)) {
	remote.Register(prefix, open)
}
