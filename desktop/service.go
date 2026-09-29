package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"dawgit/internal/agent"
	"dawgit/internal/livecheck"
	"dawgit/internal/project"
	"dawgit/internal/teams"
	"dawgit/internal/version"
)

// App is the service the frontend calls. Every method that touches a project
// takes its folder (root) and holds that project's lock, so the background
// agent and user actions never run at the same time.
type App struct {
	notify  func(title, body string)
	emit    func(name string, data any)
	mu      sync.Mutex // guards locks, agents
	locks   map[string]*sync.Mutex
	agents  map[string]context.CancelFunc
	pickDir func(title string) (string, error)
}

func NewApp() *App {
	return &App{locks: map[string]*sync.Mutex{}, agents: map[string]context.CancelFunc{}}
}

func (a *App) ServiceName() string { return "App" }

// Version is the DAWGit release number.
func (a *App) Version() string { return version.Version }

// ServiceStartup starts an agent for every downloaded team project (in all
// teams, so notices keep coming whichever team is selected).
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.migrateLegacyConfig()
	store, err := teams.Load()
	if err != nil {
		return nil
	}
	for _, root := range store.Roots() {
		a.startAgent(root)
	}
	return nil
}

func (a *App) ServiceShutdown() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.agents {
		cancel()
	}
	return nil
}

func (a *App) lock(root string) func() {
	a.mu.Lock()
	l, ok := a.locks[root]
	if !ok {
		l = &sync.Mutex{}
		a.locks[root] = l
	}
	a.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (a *App) open(root string) (*project.Repo, func(), error) {
	unlock := a.lock(root)
	r, err := project.Open(root)
	if err != nil {
		unlock()
		return nil, nil, err
	}
	var done func()
	r.OnProgress, done = a.progressFor(r.Root)
	return r, func() { done(); unlock() }, nil
}

// ProgressEvent tells the frontend how a long step (save, upload, download)
// is going.
type ProgressEvent struct {
	Root  string `json:"root"`
	Stage string `json:"stage"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// progressFor emits "progress" events for root, at most every 150 ms unless
// the stage changes. Call done when the operation ends: if anything was
// reported, it sends a final "done" event.
func (a *App) progressFor(root string) (report func(project.Progress), done func()) {
	var last time.Time
	stage := ""
	report = func(p project.Progress) {
		if a.emit == nil || (p.Stage == stage && time.Since(last) < 150*time.Millisecond) {
			return
		}
		last, stage = time.Now(), p.Stage
		a.emit("progress", ProgressEvent{Root: root, Stage: p.Stage, Done: p.Done, Total: p.Total})
	}
	done = func() {
		if a.emit != nil && stage != "" {
			a.emit("progress", ProgressEvent{Root: root, Stage: "done"})
		}
	}
	return report, done
}

// Signature changes when a set in the project is saved (or the project moves
// to another version). It only looks at file sizes and times, and takes no
// lock, so the frontend polls it to notice Ctrl+S in Live right away.
func (a *App) Signature(root string) string {
	r, err := project.Open(root)
	if err != nil {
		return ""
	}
	return r.SetsSignature()
}

// --- agent ---

type AgentEvent struct {
	Root     string    `json:"root"`
	Kind     string    `json:"kind"`
	Author   string    `json:"author"`
	Labels   []string  `json:"labels"`
	Text     string    `json:"text"`
	Versions []Version `json:"versions"`
}

func (a *App) startAgent(root string) {
	r, err := project.Open(root)
	if err != nil || r.Config.Remote == nil {
		return
	}
	a.mu.Lock()
	if _, running := a.agents[root]; running {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.agents[root] = cancel
	a.mu.Unlock()

	go func() {
		w := agent.New(root)
		t := time.NewTicker(r.PollInterval())
		defer t.Stop()
		for {
			unlock := a.lock(root)
			events := w.Check()
			unlock()
			for _, e := range events {
				a.handleEvent(root, r.Config.Name, e)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func (a *App) stopAgent(root string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel, ok := a.agents[root]; ok {
		cancel()
		delete(a.agents, root)
	}
}

func (a *App) handleEvent(root, name string, e agent.Event) {
	ev := AgentEvent{Root: root, Kind: string(e.Kind), Author: e.Author, Labels: nonNil(e.Labels), Text: e.Text,
		Versions: toVersions(e.Versions, nil)}
	if a.emit != nil {
		a.emit("agent", ev)
	}
	if a.notify == nil {
		return
	}
	switch e.Kind {
	case agent.NewVersions:
		var lines []string
		for _, m := range e.Versions {
			lines = append(lines, fmt.Sprintf("%s: %s", m.Author, m.Message))
		}
		a.notify(name+": new version from the team", strings.Join(lines, "\n"))
	case agent.Overlap:
		a.notify(name+": editing the same track", strings.ToUpper(e.Text[:1])+e.Text[1:])
	}
}

// OpenInLive opens a set with its default application (Ableton Live).
func (a *App) OpenInLive(root, set string) error {
	return shellOpen(filepath.Join(root, set))
}

func (a *App) ShowFolder(root string) error {
	return shellOpen(root)
}

// --- state ---

func (a *App) State(root string) (*State, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st := &State{Root: r.Root, Name: r.Config.Name, Author: r.Config.Author, Branch: r.BranchName(),
		Head: r.Head(), LiveRunning: livecheck.Running(), Sets: []string{},
		Changes: []Change{}, MyEdits: []project.TrackEdit{}, Incoming: []Version{}, Teammates: []Teammate{},
		Overlaps: []string{}, History: []Version{}, Branches: []Branch{}}
	if st.Name == "" {
		st.Name = filepath.Base(r.Root)
	}
	sets, _ := filepath.Glob(filepath.Join(r.Root, "*.als"))
	for _, s := range sets {
		st.Sets = append(st.Sets, filepath.Base(s))
	}

	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	for _, c := range changes {
		st.Changes = append(st.Changes, Change{Path: c.Path, Status: c.Status, Details: diffLines(c.SetDiff)})
	}
	if edits, _, err := r.LocalEdits(); err == nil {
		st.MyEdits = nonNil(edits)
	}

	tips := map[string][]string{}
	if r.Config.Remote != nil {
		st.RemoteURL = r.Config.Remote.Display()
		if t, err := r.Team(); err == nil {
			st.TeamID, st.TeamName = t.ID, t.Name
		}
		branches, err := r.Branches()
		if err != nil {
			st.Offline = err.Error()
		} else {
			st.Online = true
			for _, b := range branches {
				tips[b.Head] = append(tips[b.Head], b.Name)
				br := Branch{Name: b.Name, Current: b.Current}
				if b.Latest != nil {
					v := toVersion(b.Latest, nil)
					br.Latest = &v
				}
				st.Branches = append(st.Branches, br)
			}
			if in, err := r.IncomingVersions(); err == nil {
				st.Incoming = toVersions(in, nil)
			}
			if mates, err := r.Teammates(); err == nil {
				for _, m := range mates {
					st.Teammates = append(st.Teammates, Teammate{Author: m.Author, Updated: m.Updated, Edits: nonNil(m.Edits)})
				}
				st.Overlaps = nonNil(project.Overlaps(st.MyEdits, mates))
			}
		}
	}
	log, err := r.Log()
	if err != nil {
		return nil, err
	}
	// Include versions of the team that this workspace does not have yet so
	// the history shows where the branch is.
	seen := map[string]bool{}
	for _, m := range log {
		seen[m.ID] = true
	}
	all := append([]*project.Manifest{}, log...)
	for _, v := range st.Incoming {
		if !seen[v.ID] {
			if m, err := r.Load(v.ID); err == nil {
				all = append(all, m)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time > all[j].Time })
	st.History = toVersions(all, tips)
	return st, nil
}

// --- actions ---

// liveGuard reports whether Live must close the set before rewriting files.
func liveGuard(force bool) bool { return !force && livecheck.Running() }

func conflictResult(err error) (*Result, error) {
	var c *project.MergeConflictError
	if errors.As(err, &c) {
		return &Result{Action: "conflicts", Conflicts: toConflicts(c.Conflicts), Log: []string{}, Relinked: []string{}}, nil
	}
	return nil, err
}

func syncResult(res *project.SyncResult) *Result {
	out := &Result{Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}
	if res != nil {
		out.Action, out.Log, out.Relinked = res.Action, nonNil(res.MergeLog), nonNil(res.Relinked)
	}
	return out
}

func opts(resolutions map[string]string) project.MergeOptions {
	return project.MergeOptions{Strategy: "fail", Resolutions: resolutions}
}

// Save records a version and shares it (merging the team's versions first).
func (a *App) Save(root, message string, resolutions map[string]string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("describe what changed")
	}
	if r.Config.Remote != nil && liveGuard(force) {
		if incoming, err := r.Incoming(); err == nil && incoming {
			return &Result{Action: "blocked", LiveRunning: true, Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
		}
	}
	m, res, err := r.Save(message, opts(resolutions))
	if errors.Is(err, project.ErrNoRemote) {
		out := syncResult(nil)
		out.Action = "saved-locally"
		if m == nil {
			out.Action = "nothing"
		}
		return out, nil
	}
	if err != nil {
		return conflictResult(err)
	}
	r.ReportWorkspace()
	out := syncResult(res)
	if m == nil && res.Action == "up-to-date" {
		out.Action = "nothing"
	}
	return out, nil
}

func toPreview(p *project.Preview) *Preview {
	out := &Preview{Action: p.Action, Versions: toVersions(p.Versions, nil), Changes: []Change{},
		Conflicts: toConflicts(p.Conflicts)}
	for _, c := range p.Changes {
		out.Changes = append(out.Changes, Change{Path: c.Path, Status: c.Status, Details: diffLines(c.SetDiff)})
	}
	return out
}

func (a *App) PreviewUpdate(root string) (*Preview, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := r.PreviewUpdate()
	if err != nil {
		return nil, err
	}
	return toPreview(p), nil
}

// Update brings in the team's latest versions.
func (a *App) Update(root string, resolutions map[string]string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if liveGuard(force) {
		return &Result{Action: "blocked", LiveRunning: true, Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
	}
	res, err := r.Update(opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	r.ReportWorkspace()
	return syncResult(res), nil
}

func (a *App) CreateBranch(root, name string) error {
	r, unlock, err := a.open(root)
	if err != nil {
		return err
	}
	defer unlock()
	return r.CreateBranch(strings.TrimSpace(name))
}

func (a *App) SwitchBranch(root, name string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if liveGuard(force) {
		return &Result{Action: "blocked", LiveRunning: true, Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
	}
	res, err := r.SwitchBranch(name, false)
	if err != nil {
		return nil, err
	}
	r.ReportWorkspace()
	return syncResult(res), nil
}

func (a *App) PreviewMerge(root, name string) (*Preview, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := r.PreviewMerge(name)
	if err != nil {
		return nil, err
	}
	return toPreview(p), nil
}

func (a *App) MergeBranch(root, name string, resolutions map[string]string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if liveGuard(force) {
		return &Result{Action: "blocked", LiveRunning: true, Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
	}
	res, err := r.MergeBranch(name, opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	r.ReportWorkspace()
	return syncResult(res), nil
}
