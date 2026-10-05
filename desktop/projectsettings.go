package desktop

import (
	"errors"
	"strings"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// ProjectInfo is what a project's menu and settings show, read without
// looking at its files (fast).
type ProjectInfo struct {
	Root     string    `json:"root"`
	Name     string    `json:"name"`
	ID       string    `json:"id"`
	Branch   string    `json:"branch"`
	Tool     string    `json:"tool"`     // the program it is made with ("" unknown)
	Openable []string  `json:"openable"` // what "Open in <Tool>" offers
	Rules    RulesInfo `json:"rules"`
}

// ProjectInfo reads a project folder's name, tool and rules.
func (a *App) ProjectInfo(root string) (*ProjectInfo, error) {
	if !knownProject(root) {
		return nil, errors.New("unknown project")
	}
	r, err := project.Open(root)
	if err != nil {
		return nil, err
	}
	info := &ProjectInfo{Root: r.Root, Name: r.Config.Name, ID: r.Config.ProjectID, Branch: r.BranchName(),
		Openable: []string{}}
	rules, err := r.Profile()
	if err != nil {
		info.Rules.Error = err.Error()
	}
	if rules != nil {
		info.Rules.Applied, info.Rules.FromFile = rules.Applied(), rules.FromFile
		info.Tool = rules.Tool()
		if o, _ := rules.Openable(r.Root); o != nil {
			info.Openable = o
		}
		if err := r.CheckRules(); err != nil {
			info.Rules.Error = err.Error()
		}
	}
	return info, nil
}

// RenameProject gives a project another name, in DAWGit and for its team;
// the folder keeps its own. root: the folder when the project is here (""
// for one only on the team: teamID and projectID say which).
func (a *App) RenameProject(teamID, projectID, root, name string) error {
	name = strings.TrimSpace(name)
	if root != "" {
		r, unlock, err := a.open(root)
		if err != nil {
			return err
		}
		defer unlock()
		return r.Rename(name)
	}
	if name == "" || len([]rune(name)) > 100 {
		return errors.New("a name needs 1 to 100 characters")
	}
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(teamID)
	if t == nil {
		return errors.New("unknown team")
	}
	b, err := t.Open()
	if err != nil {
		return err
	}
	return remote.RenameProject(b, projectID, name)
}

// adoptName takes the team's name for a project here (someone renamed it).
func (a *App) adoptName(root, name string) {
	r, unlock, err := a.open(root)
	if err != nil {
		return
	}
	defer unlock()
	if r.Config.Name != name {
		r.Config.Name = name
		r.SaveConfig()
	}
}
