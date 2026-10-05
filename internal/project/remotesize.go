package project

import (
	"errors"

	"dawgit/internal/manifest"
	"dawgit/internal/teams"
)

// RemoteSize is what downloading a team project brings: its latest version
// on "main" (files in the project folder and samples from outside it).
type RemoteSize struct {
	Files int
	Bytes int64
}

// SizeOnTeam reads the size of a team project's latest version (one small
// request for the branches, one for the version record).
func SizeOnTeam(t *teams.Team, projectID string) (RemoteSize, error) {
	c, err := t.Open()
	if err != nil {
		return RemoteSize{}, err
	}
	heads, err := c.Branches(projectID)
	if err != nil {
		return RemoteSize{}, err
	}
	id := heads["main"]
	if id == "" {
		return RemoteSize{}, errors.New("the project has no versions yet")
	}
	data, err := c.GetSnapshot(projectID, id)
	if err != nil {
		return RemoteSize{}, err
	}
	m, err := manifest.Parse(id, data)
	if err != nil {
		return RemoteSize{}, err
	}
	s := RemoteSize{Files: m.FileCount, Bytes: m.TotalSize}
	if m.Tree == "" { // format 1: the files are listed
		s = RemoteSize{Files: len(m.Files)}
		for _, f := range m.Files {
			s.Bytes += f.Size
		}
	}
	for _, f := range m.External {
		s.Files++
		s.Bytes += f.Size
	}
	return s, nil
}
