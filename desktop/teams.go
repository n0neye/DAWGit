package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	// Who this computer is in the team ("" until chosen).
	MemberID   string `json:"memberId"`
	MemberName string `json:"memberName"`
	// KeysUnreadable: this computer can't read the team's keys (settings
	// copied from another computer or Windows user).
	KeysUnreadable bool `json:"keysUnreadable"`
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
}

func teamSummary(t teams.Team) TeamSummary {
	return TeamSummary{ID: t.ID, Name: t.Name, Address: t.Remote.Display(), IsStorage: t.Remote.IsStorage(),
		MemberID: t.MemberID, MemberName: t.MemberName, KeysUnreadable: t.KeysUnreadable}
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
	// Every project is a team's (0.9): with "local" or no team picked, the
	// first team is shown.
	if store.Find(store.Current) == nil && len(store.Teams) > 0 {
		store.Current = store.Teams[0].ID
		store.Save()
	}
	ov := &Overview{Author: store.Author, CurrentTeam: store.Current, Teams: []TeamSummary{},
		Projects: []TeamProject{}}
	for _, t := range store.Teams {
		ov.Teams = append(ov.Teams, teamSummary(t))
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
			} else if p.Name != "" && ov.Projects[i].Name != p.Name {
				// The team's name wins (someone renamed it, or the folder is
				// gone); the copy here takes it on.
				ov.Projects[i].Name = p.Name
				if ov.Projects[i].Status == "downloaded" {
					go a.adoptName(ov.Projects[i].Root, p.Name)
				}
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

// TeamMembers lists a team's members (to pick yourself on a new computer).
func (a *App) TeamMembers(teamID string) ([]remote.Member, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	return b.Members()
}

// SetIdentity sets who this computer is in a team: an existing member
// (memberID, e.g. the same person on another computer) or a new one
// (memberID ""). The name goes to the team's member list, so everyone sees
// it on all of that member's versions, old ones included.
func (a *App) SetIdentity(teamID, memberID, name string) (TeamSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return TeamSummary{}, errors.New("a name needs 1 to 100 characters")
	}
	store, err := teams.Load()
	if err != nil {
		return TeamSummary{}, err
	}
	t := store.Find(teamID)
	if t == nil {
		return TeamSummary{}, errors.New("unknown team")
	}
	if memberID == "" {
		memberID = teams.NewID(16)
	}
	if !remote.ValidMemberID(memberID) {
		return TeamSummary{}, errors.New("invalid member id")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return TeamSummary{}, err
	}
	if err := b.PutMember(remote.Member{ID: memberID, Name: name}); err != nil &&
		!errors.Is(err, remote.ErrOldServer) { // old server: the name still goes with new versions
		return TeamSummary{}, err
	}
	t.MemberID, t.MemberName = memberID, name
	if store.Author == "" {
		store.Author = name
	}
	if err := store.Save(); err != nil {
		return TeamSummary{}, err
	}
	forgetNames(t.Remote.URL)
	return teamSummary(*t), nil
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
	if store.Find(id) == nil {
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

// RenameTeamForEveryone changes the team's own name, on its server or
// storage; every member's DAWGit follows it.
func (a *App) RenameTeamForEveryone(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return errors.New("a team name needs 1 to 100 characters")
	}
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(id)
	if t == nil {
		return errors.New("unknown team")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return err
	}
	if err := b.SetInfo(remote.TeamInfo{Name: name}); err != nil {
		return err
	}
	t.Name, t.CustomName = name, false
	return store.Save()
}

// RemoveTeam disconnects this computer from a team. With keepProjects its
// downloaded projects move to Local (their versions stay, and they can be
// committed to here or reconnected later); otherwise they are no longer
// listed. With fullHistory the files of older versions that are only in the
// team's storage are downloaded first. Project folders always stay on disk.
func (a *App) RemoveTeam(id string, keepProjects, fullHistory bool) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	for key, root := range store.Projects {
		if !strings.HasPrefix(key, id+"/") {
			continue
		}
		a.stopAgent(root)
		if keepProjects {
			if err := a.detach(store, root, fullHistory); err != nil {
				return err
			}
		}
	}
	store.Remove(id)
	return store.Save()
}

// detach makes a downloaded team project one kept on this computer only:
// the project forgets the team's address (keeping its id, so it can be
// reconnected) and is listed under Local. Its current version is made
// complete here first, and with fullHistory every older one too (else those
// keep needing the team's storage). The caller saves the store.
func (a *App) detach(store *teams.Store, root string, fullHistory bool) error {
	a.stopAgent(root)
	r, unlock, err := a.open(root)
	if err != nil {
		for key, p := range store.Projects {
			if p == root {
				delete(store.Projects, key)
			}
		}
		return nil // the folder is gone: nothing to keep
	}
	defer unlock()
	if r.Config.Remote != nil {
		if err := r.PrepareDetach(fullHistory); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(root), err)
		}
	}
	for key, p := range store.Projects {
		if p == root {
			delete(store.Projects, key)
		}
	}
	r.Config.Remote = nil
	if err := r.SaveConfig(); err != nil {
		return err
	}
	store.AddLocal(r.Root)
	return nil
}

// FoundProject is a project on this computer that belongs to a team.
type FoundProject struct {
	Root string `json:"root"`
	Name string `json:"name"`
}

// TeamProjectsHere lists Local projects that are the team's (e.g. kept when
// this computer disconnected from it), to offer reconnecting them.
func (a *App) TeamProjectsHere(teamID string) ([]FoundProject, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	projects, err := b.Projects()
	if err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, p := range projects {
		ids[p.ID] = true
	}
	out := []FoundProject{}
	for _, root := range store.Local {
		if r, err := project.Open(root); err == nil && ids[r.Config.ProjectID] {
			name := r.Config.Name
			if name == "" {
				name = filepath.Base(root)
			}
			out = append(out, FoundProject{Root: root, Name: name})
		}
	}
	return out, nil
}

// ReconnectProjects puts Local projects back in their team (see
// TeamProjectsHere).
func (a *App) ReconnectProjects(teamID string, roots []string) error {
	for _, root := range roots {
		if _, err := a.AddProjectToTeam(teamID, root); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(root), err)
		}
	}
	return nil
}

// DownloadSize is what downloading a team project takes, shown before it.
type DownloadSize struct {
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"` // the latest version's files
	// Needed: on this disk, the files and DAWGit's own copy of them.
	Needed int64 `json:"needed"`
	Free   int64 `json:"free"` // on the disk of parent; -1 unknown
}

// ProjectDownloadSize reads the size of a team project's latest version and
// the free space where it would go (parent, or the nearest folder above it
// that exists).
func (a *App) ProjectDownloadSize(teamID, projectID, parent string) (DownloadSize, error) {
	store, err := teams.Load()
	if err != nil {
		return DownloadSize{}, err
	}
	t := store.Find(teamID)
	if t == nil {
		return DownloadSize{}, errors.New("unknown team")
	}
	s, err := project.SizeOnTeam(t, projectID)
	if err != nil {
		return DownloadSize{}, err
	}
	out := DownloadSize{Files: s.Files, Bytes: s.Bytes, Needed: 2 * s.Bytes, Free: -1}
	for dir := parent; dir != ""; {
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			out.Free = diskFree(dir)
			break
		}
		up := filepath.Dir(dir)
		if up == dir {
			break
		}
		dir = up
	}
	return out, nil
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
	folder = projectFolder(folder)
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
	release, err := r.Lock(30 * time.Second)
	if err != nil {
		unlock()
		return TeamProject{}, err
	}
	err = r.JoinTeam(t)
	release()
	unlock()
	if err != nil {
		return TeamProject{}, err
	}
	// The project now lives in this team, so show that team.
	if err := a.SelectTeam(teamID); err != nil {
		return TeamProject{}, err
	}
	a.startAgent(r.Root)
	return folderProject(r.Root, "downloaded"), nil
}

// projectFolder is the project a chosen folder means: picking the hidden
// .dawgit folder (the folder dialog opens where it was last, and lists it
// first) or a folder inside it means its project.
func projectFolder(folder string) string {
	clean := filepath.Clean(folder)
	parts := strings.Split(clean, string(filepath.Separator))
	for i, p := range parts {
		if strings.EqualFold(p, ".dawgit") && i > 0 {
			return strings.Join(parts[:i], string(filepath.Separator))
		}
	}
	return clean
}

// LocateProject points a team project at a folder that was moved.
func (a *App) LocateProject(teamID, projectID, folder string) (TeamProject, error) {
	folder = projectFolder(folder)
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
	// Deleted for everyone: first bring what isn't here yet (the copy here is
	// kept under Local with its whole history).
	if root := store.ProjectRoot(teamID, projectID); root != "" {
		if err := a.detach(store, root, true); err != nil {
			return err
		}
		if err := store.Save(); err != nil {
			return err
		}
	}
	if err := b.DeleteProject(projectID); err != nil {
		return err
	}
	store.ForgetProject(teamID, projectID)
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

// HistoryDownloadSize is how much the files of older versions that are only
// in the team's storage weigh, for one project (root) or for every project
// of a team (teamID), to offer downloading them when leaving the team.
func (a *App) HistoryDownloadSize(root, teamID string) (int64, error) {
	roots := []string{root}
	if teamID != "" {
		store, err := teams.Load()
		if err != nil {
			return 0, err
		}
		roots = nil
		for key, r := range store.Projects {
			if strings.HasPrefix(key, teamID+"/") {
				roots = append(roots, r)
			}
		}
	}
	var total int64
	for _, rt := range roots {
		if r, err := project.Open(rt); err == nil {
			_, size, _ := r.HistoryNotHere()
			total += size
		}
	}
	return total, nil
}
