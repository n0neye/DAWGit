package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// ChooseFolder asks the user for a folder ("" when cancelled).
func (a *App) ChooseFolder(title string) (string, error) {
	if a.pickDir == nil {
		return "", errors.New("folder picker not available")
	}
	return a.pickDir(title)
}

// --- data for the frontend ---

type TeamSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	IsStorage bool   `json:"isStorage"`
}

// TeamProject is a project as the sidebar shows it.
type TeamProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Root string `json:"root"`
	// Status: "downloaded" (folder here), "remote" (on the team only),
	// "missing" (downloaded before, folder not found), "local" (no team).
	Status string `json:"status"`
	Branch string `json:"branch"`
}

type Overview struct {
	Author      string        `json:"author"`
	Teams       []TeamSummary `json:"teams"`
	CurrentTeam string        `json:"currentTeam"`
	Projects    []TeamProject `json:"projects"` // of the current team
	TeamError   string        `json:"teamError"`
	Local       []TeamProject `json:"local"` // kept on this computer only
}

func teamSummary(t teams.Team) TeamSummary {
	return TeamSummary{ID: t.ID, Name: t.Name, Address: t.Remote.Display(), IsStorage: t.Remote.IsStorage()}
}

func folderProject(root, status string) TeamProject {
	p := TeamProject{Root: root, Status: status, Name: filepath.Base(root)}
	if r, err := project.Open(root); err == nil && r.Root == root {
		p.ID, p.Name, p.Branch = r.Config.ProjectID, r.Config.Name, r.BranchName()
	} else if status != "remote" {
		p.Status = "missing"
	}
	return p
}

// Overview is everything the sidebar and onboarding need.
func (a *App) Overview() (*Overview, error) {
	a.migrateLegacyConfig()
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	ov := &Overview{Author: store.Author, CurrentTeam: store.Current, Teams: []TeamSummary{},
		Projects: []TeamProject{}, Local: []TeamProject{}}
	for _, t := range store.Teams {
		ov.Teams = append(ov.Teams, teamSummary(t))
	}
	for _, root := range store.Local {
		ov.Local = append(ov.Local, folderProject(root, "local"))
	}
	t := store.Find(store.Current)
	if t == nil {
		return ov, nil
	}
	// Downloaded projects first (known even when offline), then the team's list.
	seen := map[string]int{} // project id -> index in ov.Projects
	prefix := t.ID + "/"
	for key, root := range store.Projects {
		if strings.HasPrefix(key, prefix) {
			p := folderProject(root, "downloaded")
			if p.ID == "" {
				p.ID = strings.TrimPrefix(key, prefix)
			}
			seen[p.ID] = len(ov.Projects)
			ov.Projects = append(ov.Projects, p)
		}
	}
	b, err := remote.Open(t.Remote)
	if err == nil {
		// Follow the team's name when whoever runs it renames it.
		if info, err := b.Info(); err == nil && store.SyncName(t.ID, info.Name) && store.Save() == nil {
			for i := range ov.Teams {
				if ov.Teams[i].ID == t.ID {
					ov.Teams[i].Name = info.Name
				}
			}
		}
	}
	if err != nil {
		ov.TeamError = err.Error()
	} else if ps, err := b.Projects(); err != nil {
		ov.TeamError = err.Error()
	} else {
		for _, p := range ps {
			if i, ok := seen[p.ID]; !ok {
				ov.Projects = append(ov.Projects, TeamProject{ID: p.ID, Name: p.Name, Status: "remote"})
			} else if ov.Projects[i].Status == "missing" && p.Name != "" {
				ov.Projects[i].Name = p.Name // the folder name says little once it's gone
			}
		}
	}
	sort.SliceStable(ov.Projects, func(i, j int) bool {
		return strings.ToLower(ov.Projects[i].Name) < strings.ToLower(ov.Projects[j].Name)
	})
	return ov, nil
}

func (a *App) SetAuthor(name string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	store.Author = strings.TrimSpace(name)
	return store.Save()
}

// ConnectTeam adds a team (server address + token, or a connection code) and
// makes it the current one.
func (a *App) ConnectTeam(address, token string) (TeamSummary, error) {
	t, err := project.Connect(address, token)
	if err != nil {
		return TeamSummary{}, err
	}
	if err := a.SelectTeam(t.ID); err != nil {
		return TeamSummary{}, err
	}
	return teamSummary(*t), nil
}

func (a *App) SelectTeam(id string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	if id != teams.LocalID && store.Find(id) == nil {
		return errors.New("unknown team")
	}
	store.Current = id
	return store.Save()
}

func (a *App) RenameTeam(id, name string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(id)
	if t == nil {
		return errors.New("unknown team")
	}
	if err := store.Rename(t.ID, name); err != nil {
		return err
	}
	if !t.CustomName { // back to the team's own name
		if b, err := remote.Open(t.Remote); err == nil {
			if info, err := b.Info(); err == nil {
				store.SyncName(t.ID, info.Name)
			}
		}
	}
	return store.Save()
}

// RemoveTeam disconnects this computer from a team. Project folders stay on
// disk; they are no longer listed or watched.
func (a *App) RemoveTeam(id string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	for key, root := range store.Projects {
		if strings.HasPrefix(key, id+"/") {
			a.stopAgent(root)
		}
	}
	store.Remove(id)
	return store.Save()
}

// DownloadProject downloads a team project into parent/<name> Project.
func (a *App) DownloadProject(teamID, projectID, parent string) (TeamProject, error) {
	store, err := teams.Load()
	if err != nil {
		return TeamProject{}, err
	}
	t := store.Find(teamID)
	if t == nil {
		return TeamProject{}, errors.New("unknown team")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return TeamProject{}, err
	}
	list, err := b.Projects()
	if err != nil {
		return TeamProject{}, err
	}
	name := ""
	for _, p := range list {
		if p.ID == projectID {
			name = p.Name
		}
	}
	if name == "" {
		return TeamProject{}, errors.New("the project is no longer on the team")
	}
	dir := filepath.Join(parent, name+" Project")
	report, done := a.progressFor(dir)
	r, _, err := project.CloneFromTeam(t, projectID, dir, store.Author, report)
	done()
	if err != nil {
		return TeamProject{}, err
	}
	a.startAgent(r.Root)
	return folderProject(r.Root, "downloaded"), nil
}

// AddProjectToTeam starts tracking an Ableton project folder as part of a
// team. It returns quickly; the frontend then commits and uploads the first
// version with Save, showing its progress.
func (a *App) AddProjectToTeam(teamID, folder string) (TeamProject, error) {
	store, err := teams.Load()
	if err != nil {
		return TeamProject{}, err
	}
	t := store.Find(teamID)
	if t == nil {
		return TeamProject{}, errors.New("unknown team")
	}
	r, err := project.Open(folder)
	switch {
	case errors.Is(err, project.ErrNotRepo):
		if r, err = project.Init(folder, store.Author); err != nil {
			return TeamProject{}, err
		}
	case err != nil:
		return TeamProject{}, err
	case r.Config.Remote != nil && teams.NormalizeURL(r.Config.Remote.URL) != t.Remote.URL:
		return TeamProject{}, fmt.Errorf("this project already belongs to another team (%s)", r.Config.Remote.URL)
	}
	unlock := a.lock(r.Root)
	if err := r.JoinTeam(t); err != nil {
		unlock()
		return TeamProject{}, err
	}
	unlock()
	// The project now lives in this team, so show that team.
	if err := a.SelectTeam(teamID); err != nil {
		return TeamProject{}, err
	}
	a.startAgent(r.Root)
	return folderProject(r.Root, "downloaded"), nil
}

// AddLocalProject tracks a folder on this computer only (no team).
func (a *App) AddLocalProject(folder string) (TeamProject, error) {
	store, err := teams.Load()
	if err != nil {
		return TeamProject{}, err
	}
	r, err := project.Open(folder)
	if errors.Is(err, project.ErrNotRepo) {
		r, err = project.Init(folder, store.Author)
	}
	if err != nil {
		return TeamProject{}, err
	}
	if r.Config.Remote != nil {
		return TeamProject{}, errors.New("this project belongs to a team: connect to that team to open it")
	}
	store.AddLocal(r.Root)
	store.Current = teams.LocalID // show it
	if err := store.Save(); err != nil {
		return TeamProject{}, err
	}
	return folderProject(r.Root, "local"), nil
}

// ShareProject moves a local-only project into a team (see AddProjectToTeam).
func (a *App) ShareProject(root, teamID string) (TeamProject, error) {
	return a.AddProjectToTeam(teamID, root)
}

// LocateProject points a team project at a folder that was moved.
func (a *App) LocateProject(teamID, projectID, folder string) (TeamProject, error) {
	r, err := project.Open(folder)
	if err != nil {
		return TeamProject{}, err
	}
	if r.Config.ProjectID != projectID {
		return TeamProject{}, errors.New("that folder holds a different project")
	}
	store, err := teams.Load()
	if err != nil {
		return TeamProject{}, err
	}
	store.SetProjectRoot(teamID, projectID, r.Root)
	if err := store.Save(); err != nil {
		return TeamProject{}, err
	}
	a.startAgent(r.Root)
	return folderProject(r.Root, "downloaded"), nil
}

// ForgetProject removes a project from the list (the folder is untouched).
func (a *App) ForgetProject(root string) error {
	a.stopAgent(root)
	store, err := teams.Load()
	if err != nil {
		return err
	}
	for key, r := range store.Projects {
		if r == root {
			delete(store.Projects, key)
		}
	}
	store.RemoveLocal(root)
	return store.Save()
}

// DeleteProjectFromTeam removes a project from the team's server or storage
// for everyone. The copy on this computer (if any) is kept, with its history,
// as a project on this computer only.
func (a *App) DeleteProjectFromTeam(teamID, projectID string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(teamID)
	if t == nil {
		return errors.New("unknown team")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return err
	}
	if err := b.DeleteProject(projectID); err != nil {
		return err
	}
	root := store.ProjectRoot(teamID, projectID)
	store.ForgetProject(teamID, projectID)
	if root != "" {
		a.stopAgent(root)
		unlock := a.lock(root)
		defer unlock()
		if r, err := project.Open(root); err == nil {
			r.Config.Remote = nil
			if err := r.SaveConfig(); err != nil {
				return err
			}
			store.AddLocal(r.Root)
		}
	}
	return store.Save()
}

// ServerProjects lists a team's projects before connecting (onboarding).
func (a *App) ServerProjects(address, token string) ([]remote.Project, error) {
	cfg, err := remote.ParseAddress(address, token)
	if err != nil {
		return nil, err
	}
	b, err := remote.Open(cfg)
	if err != nil {
		return nil, err
	}
	ps, err := b.Projects()
	return nonNil(ps), err
}

// migrateLegacyConfig moves the project list of DAWGit 0.1 (desktop.json)
// into the team store once.
func (a *App) migrateLegacyConfig() {
	old := loadConfig()
	if len(old.Projects) == 0 {
		return
	}
	// Team projects first: Repo.Team moves their credentials into the store
	// (and records the folder), saving the store itself.
	var local []string
	for _, root := range old.Projects {
		r, err := project.Open(root)
		if err != nil {
			continue
		}
		if r.Config.Remote == nil {
			local = append(local, r.Root)
			continue
		}
		if t, err := r.Team(); err == nil {
			if store, err := teams.Load(); err == nil {
				store.SetProjectRoot(t.ID, r.Config.ProjectID, r.Root)
				store.Save()
			}
		}
	}
	store, err := teams.Load()
	if err != nil {
		return
	}
	for _, root := range local {
		store.AddLocal(root)
	}
	if store.Save() == nil {
		os.Rename(configPath(), configPath()+".migrated")
	}
}
