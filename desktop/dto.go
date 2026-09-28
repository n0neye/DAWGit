package main

import (
	"strings"

	"dawgit/internal/diff"
	"dawgit/internal/project"
)

// Data sent to the frontend. Field names become the TypeScript model.

type ProjectSummary struct {
	Root      string `json:"root"`
	Name      string `json:"name"`
	Branch    string `json:"branch"`
	RemoteURL string `json:"remoteUrl"`
	Error     string `json:"error"`
}

type Version struct {
	ID       string   `json:"id"`
	Short    string   `json:"short"`
	Author   string   `json:"author"`
	Time     string   `json:"time"` // RFC 3339
	Message  string   `json:"message"`
	Parents  []string `json:"parents"`
	Branches []string `json:"branches"` // server branches whose latest version this is
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
	Root        string   `json:"root"`
	Name        string   `json:"name"`
	Author      string   `json:"author"`
	Branch      string   `json:"branch"`
	RemoteURL   string   `json:"remoteUrl"`
	Online      bool     `json:"online"`
	Offline     string   `json:"offline"` // why the server is unreachable
	LiveRunning bool     `json:"liveRunning"`
	Head        string   `json:"head"`
	Sets        []string `json:"sets"` // .als files in the project folder

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
	// LiveRunning: Ableton Live must close the set first; call again with
	// force when it is not open.
	LiveRunning bool `json:"liveRunning"`
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
		Parents: parents, Branches: tips[m.ID]}
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
