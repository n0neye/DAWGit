package main

import (
	"strings"

	"dawgit/internal/diff"
	"dawgit/internal/project"
)

// Data sent to the frontend. Field names become the TypeScript model.

type Version struct {
	ID       string   `json:"id"`
	Short    string   `json:"short"`
	Author   string   `json:"author"`
	Time     string   `json:"time"` // RFC 3339
	Message  string   `json:"message"`
	Parents  []string `json:"parents"`
	Branches []string `json:"branches"` // server branches whose latest version this is
	AuthorID string   `json:"authorId"` // team member id ("" for older versions)
	// InBranch: the current branch already contains this version (only set
	// in State.History); other versions can be merged in.
	InBranch bool `json:"inBranch"`
}

type Change struct {
	Path    string   `json:"path"`
	Status  string   `json:"status"`  // added | modified | deleted
	Details []string `json:"details"` // semantic diff lines for sets
}

type Teammate struct {
	Author  string              `json:"author"`
	Updated string              `json:"updated"`
	Edits   []project.TrackEdit `json:"edits"`
}

type Branch struct {
	Name    string   `json:"name"`
	Current bool     `json:"current"`
	Latest  *Version `json:"latest"`
}

type Conflict struct {
	Key         string `json:"key"`
	File        string `json:"file"`
	Unit        string `json:"unit"`
	Description string `json:"description"`
	CanKeepBoth bool   `json:"canKeepBoth"`
}

type State struct {
	Root      string `json:"root"`
	Name      string `json:"name"`
	Author    string `json:"author"`
	Branch    string `json:"branch"`
	RemoteURL string `json:"remoteUrl"`
	TeamID    string `json:"teamId"` // "" for a project kept on this computer only
	TeamName  string `json:"teamName"`
	// TeamChecked: the team fields below come from a TeamState call (false
	// until one ran for this project since DAWGit started).
	TeamChecked bool     `json:"teamChecked"`
	Online      bool     `json:"online"`
	Offline     string   `json:"offline"`     // why the server is unreachable
	LiveRunning bool     `json:"liveRunning"` // a set of this project is open in Live
	Head        string   `json:"head"`
	Sets        []string `json:"sets"` // .als files in the project folder
	// OlderVersion is the version the project was moved back to (Go to
	// version); nil on the latest version. Latest is the branch's newest.
	OlderVersion *Version `json:"olderVersion"`
	Latest       string   `json:"latest"`

	Changes   []Change            `json:"changes"`
	MyEdits   []project.TrackEdit `json:"myEdits"`
	Incoming  []Version           `json:"incoming"`
	Teammates []Teammate          `json:"teammates"`
	Overlaps  []string            `json:"overlaps"`
	History   []Version           `json:"history"`
	Branches  []Branch            `json:"branches"`
}

// Result of save / update / merge / switch.
type Result struct {
	Action   string   `json:"action"`
	Log      []string `json:"log"`
	Relinked []string `json:"relinked"`
	// Conflicts need decisions; nothing was changed. Call again with
	// resolutions keyed by Conflict.Key.
	Conflicts []Conflict `json:"conflicts"`
	// LiveRunning: a set of this project is open in Ableton Live and must be
	// closed first (OpenSet names it; "" when it cannot be told); call again
	// with force when it is closed.
	LiveRunning bool   `json:"liveRunning"`
	OpenSet     string `json:"openSet"`
}

type Preview struct {
	Action    string     `json:"action"` // up-to-date | ahead | fast-forward | merge
	Versions  []Version  `json:"versions"`
	Changes   []Change   `json:"changes"`
	Conflicts []Conflict `json:"conflicts"`
}

func toVersion(m *project.Manifest, tips map[string][]string) Version {
	parents := m.Parents
	if parents == nil {
		parents = []string{}
	}
	return Version{ID: m.ID, Short: m.ID[:10], Author: m.Author, Time: m.Time, Message: m.Message,
		Parents: parents, Branches: tips[m.ID], AuthorID: m.AuthorID}
}

// renameAuthors shows members' current names (a rename applies to
// everything they did).
func renameAuthors(names map[string]string, vs []Version) {
	for i := range vs {
		if n := names[vs[i].AuthorID]; n != "" {
			vs[i].Author = n
		}
	}
}

func toVersions(ms []*project.Manifest, tips map[string][]string) []Version {
	out := []Version{}
	for _, m := range ms {
		out = append(out, toVersion(m, tips))
	}
	return out
}

func diffLines(d *diff.SetDiff) []string {
	if d == nil || d.Empty() {
		return []string{}
	}
	return strings.Split(d.Render(), "\n")
}

func toConflicts(cs []project.ConflictItem) []Conflict {
	out := []Conflict{}
	for _, c := range cs {
		out = append(out, Conflict{Key: c.Key, File: c.File, Unit: c.Unit, Description: c.Description,
			CanKeepBoth: c.CanKeepBoth})
	}
	return out
}

func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}
