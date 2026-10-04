package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"dawgit/internal/blob"
	"dawgit/internal/manifest"
	"dawgit/internal/project"
	"dawgit/internal/remote"
)

// Output for programs and AI agents: with --json, a command that supports
// it prints one JSON object on stdout and nothing else (warnings go to
// stderr):
//
//	{"schema": 1, "ok": true, "command": "status", "result": {...}}
//	{"schema": 1, "ok": false, "command": "save", "error": {"code": "merge_conflict", ...}}
//
// Errors have a fixed code (see the table in docs/agents.md) and an exit
// status by kind: 2 usage, 3 merge conflict, 4 a set is open in Live, 5 try
// again later, 6 needs another DAWGit, 1 anything else. Commands never wait
// for an answer: where one is needed they fail with a code.

const jsonSchema = 1

// jsonMode: --json was given.
var jsonMode bool

// jsonCommands support --json.
var jsonCommands = map[string]bool{"status": true, "log": true, "save": true, "update": true, "merge": true,
	"version": true, "backup": true}

// stripJSON takes --json (or -json) out of args.
func stripJSON(args []string) ([]string, bool) {
	var out []string
	found := false
	for _, a := range args {
		if a == "--json" || a == "-json" {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

type envelope struct {
	Schema  int        `json:"schema"`
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Result  any        `json:"result,omitempty"`
	Error   *errorJSON `json:"error,omitempty"`
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	os.Stdout.Write(append(data, '\n'))
}

// result prints a command's result: as JSON with --json, else with text.
func result(command string, v any, text func()) {
	if jsonMode {
		printJSON(envelope{Schema: jsonSchema, OK: true, Command: command, Result: v})
		return
	}
	text()
}

// Exit statuses.
const (
	exitError    = 1
	exitUsage    = 2
	exitConflict = 3
	exitLiveOpen = 4
	exitLater    = 5
	exitUpgrade  = 6
)

// cliError is an error with its code (for --json and the exit status).
type cliError struct {
	Code      string
	Exit      int
	Err       error
	Hint      string
	Conflicts []conflictJSON
	Set       string // the set open in Live
}

func (e *cliError) Error() string { return e.Err.Error() }
func (e *cliError) Unwrap() error { return e.Err }

type errorJSON struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Hint      string         `json:"hint,omitempty"`
	Exit      int            `json:"exit"`
	Conflicts []conflictJSON `json:"conflicts,omitempty"`
	Set       string         `json:"set,omitempty"`
}

func usageError(format string, a ...any) error {
	return &cliError{Code: "usage", Exit: exitUsage, Err: fmt.Errorf(format, a...)}
}

// codes for errors from the project, by sentinel.
var codes = []struct {
	err  error
	code string
	exit int
	hint string
}{
	{project.ErrNotRepo, "not_a_project", exitUpgrade, "run it in a project folder DAWGit tracks (dawgit init, or dawgit clone)"},
	{project.ErrBusy, "project_busy", exitLater, "the app or another command is working on the project: try again in a moment"},
	{manifest.ErrNewerFormat, "newer_version_needed", exitUpgrade, "install the current DAWGit"},
	{blob.ErrNewerFormat, "newer_version_needed", exitUpgrade, "install the current DAWGit"},
	{remote.ErrOldServer, "server_out_of_date", exitUpgrade, "whoever runs the team's server should install the current DAWGit"},
	{project.ErrNoRemote, "not_connected", exitError, "connect the project to a team: dawgit remote <address>"},
	{project.ErrDirty, "unsaved_changes", exitError, "save them first: dawgit save -m \"...\" (it also brings in the team's versions)"},
	{project.ErrOlderVersion, "on_older_version", exitError, "go back to the latest version: dawgit checkout latest"},
	{project.ErrUnshared, "unshared_versions", exitError, "share them first: dawgit save -m ..."},
	{project.ErrNotHere, "files_not_here", exitError, "join the team again to get the files"},
}

// classify gives err its code.
func classify(err error) *cliError {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce
	}
	var mc *project.MergeConflictError
	if errors.As(err, &mc) {
		return &cliError{Code: "merge_conflict", Exit: exitConflict, Err: err, Conflicts: conflictsJSON(mc.Conflicts),
			Hint: "choose how to resolve: --strategy ours | theirs | both"}
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			return &cliError{Code: c.code, Exit: c.exit, Err: err, Hint: c.hint}
		}
	}
	if remote.Transient(err) {
		return &cliError{Code: "team_unreachable", Exit: exitLater, Err: err,
			Hint: "the team's server or storage can't be reached: try again later"}
	}
	msg := err.Error()
	if strings.HasPrefix(msg, "usage:") || errors.Is(err, flag.ErrHelp) {
		return &cliError{Code: "usage", Exit: exitUsage, Err: err}
	}
	for _, p := range []string{"flag provided but not defined", "flag needs an argument", "invalid value",
		"invalid boolean", "a message is required"} {
		if strings.Contains(msg, p) {
			return &cliError{Code: "usage", Exit: exitUsage, Err: err}
		}
	}
	return &cliError{Code: "error", Exit: exitError, Err: err}
}

// fail reports err (text on stderr, or JSON on stdout) and returns the exit
// status.
func fail(command string, err error) int {
	ce := classify(err)
	if jsonMode {
		printJSON(envelope{Schema: jsonSchema, OK: false, Command: command, Error: &errorJSON{Code: ce.Code,
			Message: ce.Error(), Hint: ce.Hint, Exit: ce.Exit, Conflicts: ce.Conflicts, Set: ce.Set}})
		return ce.Exit
	}
	fmt.Fprintln(os.Stderr, "error:", ce.Error())
	if ce.Code == "merge_conflict" { // the conflicts are in the message; how to go on is not
		fmt.Fprintln(os.Stderr, ce.Hint)
	}
	return ce.Exit
}

// --- results ---

type versionJSON struct {
	ID       string   `json:"id"`
	Time     string   `json:"time"`
	Author   string   `json:"author"`
	AuthorID string   `json:"author_id,omitempty"`
	Message  string   `json:"message"`
	Parents  []string `json:"parents"`
	Branches []string `json:"branches,omitempty"` // team branches at this version
}

func versionOf(m *project.Manifest, names map[string]string) versionJSON {
	parents := m.Parents
	if parents == nil {
		parents = []string{}
	}
	return versionJSON{ID: m.ID, Time: m.Time, Author: project.AuthorName(m, names), AuthorID: m.AuthorID,
		Message: m.Message, Parents: parents}
}

type changeJSON struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added, modified, deleted, renamed, untracked
	From   string `json:"from,omitempty"`
	Edited bool   `json:"edited,omitempty"` // renamed and changed
	// SetChanges: what changed in a Live Set (tracks, devices, clips…), one
	// line each, as `dawgit diff` prints it.
	SetChanges []string `json:"set_changes,omitempty"`
	// Weight: for a Live Set, its heaviest change: noise (a plugin saving its
	// own state), tidy, mix, sound or arrangement.
	Weight string `json:"weight,omitempty"`
}

type conflictJSON struct {
	Key         string `json:"key"`
	File        string `json:"file"`
	Unit        string `json:"unit"` // e.g. AudioTrack "Bass", or the file itself
	Description string `json:"description"`
	CanKeepBoth bool   `json:"can_keep_both"`
}

func conflictsJSON(cs []project.ConflictItem) []conflictJSON {
	out := []conflictJSON{}
	for _, c := range cs {
		out = append(out, conflictJSON{Key: c.Key, File: c.File, Unit: c.Unit, Description: c.Description,
			CanKeepBoth: c.CanKeepBoth})
	}
	return out
}

func setLines(render string) []string {
	var out []string
	for _, l := range strings.Split(render, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// nonNil keeps empty lists as [] in JSON.
func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}
