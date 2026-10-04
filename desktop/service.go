package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"dawgit/internal/handlers"
	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
	"dawgit/internal/teamwatch"
	"dawgit/internal/version"
)

// App is the service the frontend calls. Every method that touches a project
// takes its folder (root) and holds that project's lock, so the background
// team watch and user actions never run at the same time.
type App struct {
	notify      func(title, body string)
	emit        func(name string, data any)
	mu          sync.Mutex // guards locks, teamWatches
	locks       map[string]*sync.Mutex
	teamWatches map[string]context.CancelFunc
	hidden      atomic.Bool             // the window is in the tray or minimised
	watches     map[string]*folderWatch // guarded by mu
	pickDir     func(title string) (string, error)
	openURL     func(url string) error
	quit        func()       // ends the app (to let an update's installer replace it)
	working     atomic.Int32 // projects open for an operation right now
	// lastProgress: when a long step (save, upload, download) last said how
	// it was going (UnixNano).
	lastProgress atomic.Int64
}

func NewApp() *App {
	return &App{locks: map[string]*sync.Mutex{}, teamWatches: map[string]context.CancelFunc{}, watches: map[string]*folderWatch{}}
}

func (a *App) ServiceName() string { return "App" }

// Version is the DAWGit release number.
func (a *App) Version() string { return version.Full() }

// Edition names a build with extensions ("" for the public app).
func (a *App) Edition() string { return version.Edition }

// ServiceStartup starts a team watch for every downloaded team project (in all
// teams, so notices keep coming whichever team is selected).
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.migrateLegacyConfig()
	if ok, err := teams.ImportFromPro(); ok {
		log.Printf("settings imported from DAWGit Pro (err: %v)", err)
	}
	store, err := teams.Load()
	if err != nil {
		return nil
	}
	for _, root := range store.Roots() {
		a.startWatch(root)
	}
	go a.shareSetups(ctx)
	go a.backUpOnSchedule(ctx)
	return nil
}

func (a *App) ServiceShutdown() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.teamWatches {
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
	a.working.Add(1)
	unlock0 := a.lock(root)
	unlock := func() { unlock0(); a.working.Add(-1) }
	r, err := project.Open(root)
	if err != nil {
		unlock()
		return nil, nil, err
	}
	// Other programs (the command line, the other edition) wait their turn.
	release, err := r.Lock(30 * time.Second)
	if err != nil {
		unlock()
		return nil, nil, err
	}
	var done func()
	r.OnProgress, done = a.progressFor(r.Root)
	return r, func() { done(); release(); unlock() }, nil
}

// ProgressEvent tells the frontend how a long step (save, upload, download)
// is going.
type ProgressEvent struct {
	Root  string `json:"root"`
	Stage string `json:"stage"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
	// Transfers: bytes so far and in all (0 when not known).
	Bytes      int64 `json:"bytes"`
	TotalBytes int64 `json:"totalBytes"`
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
		a.lastProgress.Store(last.UnixNano())
		a.emit("progress", ProgressEvent{Root: root, Stage: p.Stage, Done: p.Done, Total: p.Total,
			Bytes: p.Bytes, TotalBytes: p.TotalBytes})
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

// --- team watch ---

type WatchEvent struct {
	Root     string    `json:"root"`
	Kind     string    `json:"kind"`
	Author   string    `json:"author"`
	Labels   []string  `json:"labels"`
	Text     string    `json:"text"`
	Versions []Version `json:"versions"`
}

func (a *App) startWatch(root string) {
	r, err := project.Open(root)
	if err != nil || r.Config.Remote == nil {
		return
	}
	a.mu.Lock()
	if _, running := a.teamWatches[root]; running {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.teamWatches[root] = cancel
	a.mu.Unlock()

	a.tidyLater(root) // e.g. a project from before files were kept in the team's storage
	go func() {
		w := teamwatch.New(root)
		for {
			for _, e := range w.Check() {
				a.handleEvent(root, r.Config.Name, e)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(a.pollInterval(r)):
			}
		}
	}()
}

// pollInterval is how often a team watch looks for new versions. Storage bills
// each request, so it looks every minute while the window is open and every
// five minutes from the tray; a team server is asked more often.
func (a *App) pollInterval(r *project.Repo) time.Duration {
	hidden := a.hidden.Load()
	if r.Config.Remote != nil && !r.Config.Remote.IsStorage() {
		if hidden {
			return time.Minute
		}
		return r.PollInterval()
	}
	if hidden {
		return 5 * time.Minute
	}
	return time.Minute
}

// setHidden records whether the window is hidden (in the tray) or minimised.
func (a *App) setHidden(h bool) { a.hidden.Store(h) }

func (a *App) stopWatch(root string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel, ok := a.teamWatches[root]; ok {
		cancel()
		delete(a.teamWatches, root)
	}
}

func (a *App) handleEvent(root, name string, e teamwatch.Event) {
	ev := WatchEvent{Root: root, Kind: string(e.Kind), Author: e.Author, Labels: nonNil(e.Labels), Text: e.Text,
		Versions: toVersions(e.Versions, nil)}
	if a.emit != nil {
		a.emit("team-watch", ev)
	}
	if a.notify == nil {
		return
	}
	switch e.Kind {
	case teamwatch.NewVersions:
		var names map[string]string
		if r, err := project.Open(root); err == nil {
			names = a.memberNames(r)
		}
		var lines []string
		for _, m := range e.Versions {
			lines = append(lines, fmt.Sprintf("%s: %s", project.AuthorName(m, names), m.Message))
		}
		a.notify(name+": new version from the team", strings.Join(lines, "\n"))
	}
}

// OpenInLive opens a set with its default application (Ableton Live).
func (a *App) OpenInLive(root, set string) error {
	return shellOpen(filepath.Join(root, set))
}

// OpenInTool opens one of State.Openable in the project's tool: with the
// preset's opener, or the file's own program.
func (a *App) OpenInTool(root, rel string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	r, err := project.Open(root)
	if err != nil {
		return err
	}
	rules, _ := r.Profile()
	_, openers := rules.Openable(root)
	with, ok := openers[rel]
	if !ok {
		return fmt.Errorf("%s can't be opened from DAWGit", rel)
	}
	if with != "" {
		open := handlers.Opener(with)
		if open == nil {
			return fmt.Errorf("this DAWGit can't open it (no %q)", with)
		}
		return open(root, rel)
	}
	return shellOpen(filepath.Join(root, filepath.FromSlash(rel)))
}

// CommitWarnings runs the project's pre-commit checks (e.g. a Unity asset
// without its .meta) on what would be committed; the page shows them first.
func (a *App) CommitWarnings(root string) ([]string, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	rules, _ := r.Profile()
	names := rules.Checks()
	out := []string{}
	if len(names) == 0 {
		return out, nil
	}
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	var list []handlers.Change
	for _, c := range changes {
		// Checks reason with files added and deleted: a move is both (an asset
		// moved without its .meta is still found).
		if c.Status == "renamed" {
			list = append(list, handlers.Change{Path: c.From, Status: "deleted"}, handlers.Change{Path: c.Path, Status: "added"})
			continue
		}
		list = append(list, handlers.Change{Path: c.Path, Status: c.Status})
	}
	for _, name := range names {
		if check := handlers.Check(name); check != nil {
			out = append(out, check(root, list)...)
		}
	}
	return out, nil
}

func (a *App) ShowFolder(root string) error {
	return shellOpen(root)
}

// --- state ---

func (a *App) State(root string) (*State, error) {
	sw := startWatch("State " + filepath.Base(root))
	defer sw.done()
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	sw.lap("open")
	// On the latest version of another branch (e.g. gone there before this
	// was handled): that branch is where commits go.
	if r.OnOlderVersion() && r.Config.Remote != nil {
		r.AdoptBranchAtHead()
	}
	st := &State{Root: r.Root, Name: r.Config.Name, Author: r.Config.Author, Branch: r.BranchName(),
		Head: r.Head(), LiveRunning: toolOpen(r) != "",
		Changes: []Change{}, MyEdits: []project.TrackEdit{}, Incoming: []Version{}, History: []Version{},
		Branches: []Branch{}}
	if st.Name == "" {
		st.Name = filepath.Base(r.Root)
	}
	rules, _ := r.Profile()
	st.Rules = RulesInfo{Applied: rules.Applied(), FromFile: rules.FromFile, Suggestions: suggestions(r)}
	if err := r.CheckRules(); err != nil {
		st.Rules.Error = err.Error()
	}
	st.Latest = r.Latest()
	if r.OnOlderVersion() {
		if m, err := r.Load(r.Head()); err == nil {
			v := toVersion(m, nil)
			st.OlderVersion = &v
		}
	}
	st.CloudFolder = cloudFolder(r.Root)
	if id := r.UnfinishedSwitch(); id != "" {
		if m, err := r.Header(strings.TrimPrefix(id, "work ")); err == nil {
			v := toVersion(m, nil)
			st.Unfinished = &v
		}
	}
	if rules, _ := r.Profile(); rules != nil {
		st.Tool = rules.Tool()
		st.Openable, _ = rules.Openable(r.Root)
	}
	if st.Openable == nil {
		st.Openable = []string{}
	}

	sw.lap("head")
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	sw.lap("status")
	for _, c := range changes {
		st.Changes = append(st.Changes, toChange(c.Path, c.Status, c.From, c.Edited, c.SetDiff))
	}
	st.MyEdits = nonNil(project.EditsIn(changes))
	sw.lap("edits")

	// The team's side comes from the last TeamState (no network here); the
	// page then asks TeamState for a fresh one.
	var view *project.TeamView
	if r.Config.Remote != nil {
		st.RemoteURL = r.Config.Remote.Display()
		if t, err := r.Team(); err == nil {
			st.TeamID, st.TeamName = t.ID, t.Name
		}
		var tc *teamCache
		view, tc = a.cachedTeam(root)
		if tc != nil {
			st.TeamChecked, st.Online, st.Offline = true, tc.err == "", tc.err
		}
	}
	part, err := a.teamPart(r, view, false)
	if err != nil {
		return nil, err
	}
	sw.lap("log")
	st.Branches, st.Incoming, st.History = part.Branches, part.Incoming, part.History
	if part.OlderVersion != nil {
		st.OlderVersion = part.OlderVersion
	}
	return st, nil
}

// TeamPart is the team's side of a project's state (see TeamState).
type TeamPart struct {
	Online       bool      `json:"online"`
	Offline      string    `json:"offline"`
	Branches     []Branch  `json:"branches"`
	Incoming     []Version `json:"incoming"`
	History      []Version `json:"history"` // all branches, with the team's
	OlderVersion *Version  `json:"olderVersion"`
	// Unshared: this branch has versions here and none on the team yet (the
	// project was added and not shared).
	Unshared bool `json:"unshared"`
	// Capabilities of the team's backend (locks, presence…): the app shows
	// what goes with them only when it has them.
	Capabilities remote.Capabilities `json:"capabilities"`
}

// TeamState asks the team for its branches and new versions. It runs
// without the project lock, so the page shows State at once and fills this in.
func (a *App) TeamState(root string) (*TeamPart, error) {
	sw := startWatch("TeamState " + filepath.Base(root))
	defer sw.done()
	r, err := project.Open(root)
	if err != nil {
		return nil, err
	}
	if r.Config.Remote == nil {
		return a.teamPart(r, nil, false)
	}
	view, err := r.FetchTeam()
	sw.lap("fetch")
	a.storeTeam(root, view, err)
	if err != nil {
		part, perr := a.teamPart(r, nil, false)
		if perr != nil {
			return nil, perr
		}
		part.Offline = err.Error()
		return part, nil
	}
	part, err := a.teamPart(r, view, true)
	sw.lap("log")
	if err == nil {
		if c, cerr := r.Client(); cerr == nil {
			part.Capabilities = remote.CapabilitiesOf(c)
		}
	}
	return part, err
}

// teamPart works out the team's side from a fetched view (nil: none), with
// no network except, when fetchNames, the member list (cached a minute).
func (a *App) teamPart(r *project.Repo, view *project.TeamView, fetchNames bool) (*TeamPart, error) {
	part := &TeamPart{Online: view != nil, Branches: []Branch{}, Incoming: []Version{}}
	part.Unshared = view != nil && view.Heads[r.BranchName()] == "" && r.Head() != ""
	tips := map[string][]string{}
	if view != nil {
		for _, b := range r.BranchesFrom(view.Heads) {
			tips[b.Head] = append(tips[b.Head], b.Name)
			br := Branch{Name: b.Name, Current: b.Current}
			if b.Latest != nil {
				v := toVersion(b.Latest, nil)
				br.Latest = &v
			}
			part.Branches = append(part.Branches, br)
		}
		if in, err := r.IncomingFrom(view.Heads); err == nil {
			part.Incoming = toVersions(in, nil)
		}
	}
	// The whole tree: every branch (including the team's versions of this
	// branch not taken yet), wherever this workspace is.
	var heads []string
	for h := range tips {
		if h != "" {
			heads = append(heads, h)
		}
	}
	sort.Strings(heads)
	all, err := r.LogAll(heads)
	if err != nil {
		return nil, err
	}
	part.History = toVersions(all, tips)
	if notHere := r.MissingHere(all); len(notHere) > 0 {
		for i := range part.History {
			part.History[i].NotHere = notHere[part.History[i].ID]
		}
	}
	if r.OnOlderVersion() {
		if m, err := r.Load(r.Head()); err == nil {
			v := toVersion(m, nil)
			part.OlderVersion = &v
		}
	}
	if r.Config.Remote != nil {
		var names map[string]string
		if fetchNames {
			names = a.memberNames(r)
		} else {
			names = cachedMemberNames(r)
		}
		renameAuthors(names, part.History)
		renameAuthors(names, part.Incoming)
		for _, b := range part.Branches {
			if b.Latest != nil {
				if n := names[b.Latest.AuthorID]; n != "" {
					b.Latest.Author = n
				}
			}
		}
		if v := part.OlderVersion; v != nil && names[v.AuthorID] != "" {
			v.Author = names[v.AuthorID]
		}
	}
	if in, err := r.InBranch(); err == nil {
		for i := range part.History {
			part.History[i].InBranch = in[part.History[i].ID]
		}
	}
	return part, nil
}

// teamCache keeps each project's last TeamState fetch, so State (and
// switching back to a project) shows the team's side without waiting.
type teamCache struct {
	view *project.TeamView // last good one
	err  string            // the last fetch failed: why
}

var teamViews = struct {
	sync.Mutex
	byRoot map[string]*teamCache
}{byRoot: map[string]*teamCache{}}

func (a *App) cachedTeam(root string) (*project.TeamView, *teamCache) {
	teamViews.Lock()
	defer teamViews.Unlock()
	c := teamViews.byRoot[root]
	if c == nil {
		return nil, nil
	}
	if c.err != "" {
		return nil, c
	}
	return c.view, c
}

func (a *App) storeTeam(root string, view *project.TeamView, err error) {
	teamViews.Lock()
	defer teamViews.Unlock()
	if err != nil {
		teamViews.byRoot[root] = &teamCache{err: err.Error()}
		return
	}
	teamViews.byRoot[root] = &teamCache{view: view}
}

// --- actions ---

// liveGuard returns the set of this project open in Live ("?" when Live
// runs but that cannot be told), which must be closed before DAWGit rewrites
// files; "" when it is safe or forced.
func liveGuard(r *project.Repo, force bool) string {
	if force {
		return ""
	}
	return toolOpen(r)
}

// toolOpen runs the running-tool checks of the project's rules: the set of
// this project open in Live ("?" when Live runs but that cannot be told).
func toolOpen(r *project.Repo) string {
	rules, _ := r.Profile()
	for _, name := range rules.Running() {
		if check := handlers.Running(name); check != nil {
			if open := check(r.Root); open != "" {
				return open
			}
		}
	}
	return ""
}

// blocked asks the user to close the set in Live first.
func blocked(set string) *Result {
	if set == "?" {
		set = ""
	}
	return &Result{Action: "blocked", LiveRunning: true, OpenSet: set, Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}
}

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
		out.Action, out.Log, out.Relinked, out.KeptWork = res.Action, nonNil(res.MergeLog), nonNil(res.Relinked), res.KeptWork
	}
	return out
}

func opts(resolutions map[string]string) project.MergeOptions {
	return project.MergeOptions{Strategy: "fail", Resolutions: resolutions}
}

// Save records a version and shares it (merging the team's versions first).
//
// combine: when teammates committed on this branch in the meantime, their
// versions are combined with this one. Without it such a save changes
// nothing and returns action "behind", so the user decides (combine, new
// branch, or discard) with a preview.
// Save commits (and shares) the changes; paths, when not empty, are the
// changes to commit: the others stay uncommitted.
func (a *App) Save(root, message string, combine bool, resolutions map[string]string, force bool, paths []string) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("describe what changed")
	}
	if len(paths) > 0 {
		r.Only = paths
	}
	incoming := false
	if r.Config.Remote != nil {
		incoming, _ = r.Incoming()
		// Changes made on an older version: newer versions are "incoming"
		// for them, just as when teammates committed in the meantime.
		incoming = incoming || r.OnOlderVersion()
	}
	// Teammates' versions are taken in first (Repo.Save); changes made on
	// an older version are combined only when asked.
	if r.OnOlderVersion() && !combine {
		return &Result{Action: "behind", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
	}
	if set := liveGuard(r, force); incoming && set != "" {
		return blocked(set), nil
	}
	var older *project.Manifest
	if r.OnOlderVersion() && r.Config.Remote != nil {
		if older, err = r.CommitOnOlderVersion(message); err != nil {
			return nil, err
		}
	}
	m, res, err := r.Save(message, opts(resolutions))
	if m == nil {
		m = older
	}
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
	out := syncResult(res)
	if m == nil && res.Action == "up-to-date" {
		out.Action = "nothing"
	}
	return out, nil
}

// ShareVersions shares the versions committed here with the project's team
// without committing the files (a project added to a team, shared later).
func (a *App) ShareVersions(root string) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	res, err := r.Share(opts(nil))
	if err != nil {
		return conflictResult(err)
	}
	return syncResult(res), nil
}

func toPreview(p *project.Preview, names map[string]string) *Preview {
	out := &Preview{Action: p.Action, Versions: toVersions(p.Versions, nil), Changes: []Change{},
		Conflicts: toConflicts(p.Conflicts), Base: p.Base, Target: p.Target}
	renameAuthors(names, out.Versions)
	for _, c := range p.Changes {
		out.Changes = append(out.Changes, toChange(c.Path, c.Status, c.From, c.Edited, c.SetDiff))
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
	return toPreview(p, a.memberNames(r)), nil
}

// Update brings in the team's latest versions.
func (a *App) Update(root string, resolutions map[string]string, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	res, err := r.Update(opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	return syncResult(res), nil
}

// GoToVersion puts the project folder in the state of a version ("latest"
// goes back to the newest). discard drops uncommitted changes; force goes
// ahead while Live is running.
func (a *App) GoToVersion(root, id string, discard, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	_, notes, err := r.GoTo(id, discard)
	if errors.Is(err, project.ErrDirty) {
		return nil, errors.New("you have uncommitted changes: commit or discard them first")
	}
	if err != nil {
		return nil, err
	}
	return &Result{Action: "moved", Log: []string{}, Relinked: nonNil(notes), Conflicts: []Conflict{}}, nil
}

// RecoverSwitch puts the files back as the version the project is on, after
// a switch that stopped halfway (State.Unfinished).
func (a *App) RecoverSwitch(root string, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	notes, err := r.RecoverSwitch()
	if err != nil {
		return nil, err
	}
	return &Result{Action: "moved", Log: []string{}, Relinked: nonNil(notes), Conflicts: []Conflict{}}, nil
}

// KeepThisVersion continues from the older version the project is on: it
// becomes a new version on top of the latest (shared with the team).
func (a *App) KeepThisVersion(root, message string, resolutions map[string]string) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if strings.TrimSpace(message) == "" {
		return nil, errors.New("describe the version")
	}
	if _, err := r.KeepThisVersion(message); err != nil {
		return nil, err
	}
	if r.Config.Remote == nil {
		out := syncResult(nil)
		out.Action = "saved-locally"
		return out, nil
	}
	_, res, err := r.Save(message, opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	return syncResult(res), nil
}

// ExportVersion writes a version as a separate project folder inside parent
// and returns its path.
func (a *App) ExportVersion(root, id, parent string) (string, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return "", err
	}
	defer unlock()
	full, err := r.Resolve(id)
	if err != nil {
		return "", err
	}
	m, err := r.Load(full)
	if err != nil {
		return "", err
	}
	name := r.ExportName(m)
	dir := filepath.Join(parent, name)
	for n := 2; ; n++ { // exported before: "… Project 2", "… Project 3"
		if entries, err := os.ReadDir(dir); err != nil || len(entries) == 0 {
			break
		}
		dir = filepath.Join(parent, fmt.Sprintf("%s %d", name, n))
	}
	if _, err := r.Export(full, dir); err != nil {
		return "", err
	}
	return dir, nil
}

// DiscardAndUpdate drops uncommitted changes and takes the team's latest
// versions of this branch.
func (a *App) DiscardAndUpdate(root string, resolutions map[string]string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	if r.OnOlderVersion() {
		return nil, project.ErrOlderVersion
	}
	if head := r.Head(); head != "" {
		if _, _, err := r.Checkout(head, true); err != nil {
			return nil, err
		}
	}
	res, err := r.Update(opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
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
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	res, err := r.SwitchBranch(name, false)
	if err != nil {
		return nil, err
	}
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
	out := toPreview(p, a.memberNames(r))
	out.Message = "Merge branch " + name
	return out, nil
}

// PreviewMergeVersion previews merging any version (e.g. one in the middle
// of another branch) into the current branch.
func (a *App) PreviewMergeVersion(root, id string) (*Preview, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	p, err := r.PreviewVersion(id)
	if err != nil {
		return nil, err
	}
	out := toPreview(p, a.memberNames(r))
	out.Message, _ = r.MergeMessage(id)
	return out, nil
}

// VersionChanges lists what a version changed compared with the one before
// it (for a merge: what it brought into its branch), for the History tab.
func (a *App) VersionChanges(root, id string) ([]Change, error) {
	r, err := project.Open(root) // reads stored versions only: no lock
	if err != nil {
		return nil, err
	}
	// A teammate's version not taken in yet: its sets are downloaded first
	// (they are small, and kept), under the project's lock.
	if missing, err := r.MissingSets(id); err == nil && len(missing) > 0 && r.Config.Remote != nil {
		lr, unlock, err := a.open(root)
		if err != nil {
			return nil, err
		}
		err = lr.FetchSets(missing)
		unlock()
		if err != nil {
			return nil, err
		}
	}
	changes, err := r.VersionChanges(id)
	if err != nil {
		return nil, err
	}
	out := []Change{}
	for _, c := range changes {
		out = append(out, toChange(c.Path, c.Status, c.From, c.Edited, c.SetDiff))
	}
	return out, nil
}

// MergeVersion merges a version into the current branch; message describes
// the merge version ("" for the default, see Preview.Message).
func (a *App) MergeVersion(root, id, message string, resolutions map[string]string, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	res, err := r.MergeVersion(id, message, opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	return syncResult(res), nil
}

func (a *App) MergeBranch(root, name, message string, resolutions map[string]string, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if set := liveGuard(r, force); set != "" {
		return blocked(set), nil
	}
	res, err := r.MergeBranch(name, message, opts(resolutions))
	if err != nil {
		return conflictResult(err)
	}
	return syncResult(res), nil
}
