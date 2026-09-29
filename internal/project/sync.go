package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// ErrNoRemote is returned by sync operations for a project kept on this
// computer only.
var ErrNoRemote = errors.New("not connected to a team (run `dawgit remote <address>`)")

// Team returns the team this project belongs to, with credentials from the
// per-user team store. Credentials found in an older project config are
// moved to the store and removed from the project folder.
func (r *Repo) Team() (*teams.Team, error) {
	if r.Config.Remote == nil || r.Config.Remote.URL == "" {
		return nil, ErrNoRemote
	}
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	cfg := *r.Config.Remote
	if cfg.Token != "" || cfg.AccessKey != "" || cfg.SecretKey != "" {
		t := store.Upsert(cfg, "")
		store.SetProjectRoot(t.ID, r.Config.ProjectID, r.Root)
		if err := store.Save(); err != nil {
			return nil, err
		}
		r.Config.Remote = &RemoteConfig{URL: t.Remote.URL}
		if err := r.SaveConfig(); err != nil {
			return nil, err
		}
	}
	t := store.FindByURL(cfg.URL)
	if t == nil {
		return nil, fmt.Errorf("this computer is not connected to the team at %s (run `dawgit remote %s --token ...`)",
			cfg.URL, cfg.URL)
	}
	return t, nil
}

func (r *Repo) Client() (remote.Backend, error) {
	t, err := r.Team()
	if err != nil {
		return nil, err
	}
	return remote.Open(t.Remote)
}

// SetRemote connects the project to a team: a server (address + token) or
// storage (a connection code as address). Credentials go to the per-user
// team store; the project only records the team's address.
func (r *Repo) SetRemote(address, token string) error {
	t, err := Connect(address, token)
	if err != nil {
		return err
	}
	return r.JoinTeam(t)
}

// JoinTeam makes the project belong to t (already in the team store).
func (r *Repo) JoinTeam(t *teams.Team) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	store.SetProjectRoot(t.ID, r.Config.ProjectID, r.Root)
	if err := store.Save(); err != nil {
		return err
	}
	r.Config.Remote = &RemoteConfig{URL: t.Remote.URL}
	return r.SaveConfig()
}

// Connect checks a team address (and token or connection code), records the
// team in the per-user store with the name the team gives itself, and
// returns it.
func Connect(address, token string) (*teams.Team, error) {
	cfg, err := remote.ParseAddress(address, token)
	if err != nil {
		return nil, err
	}
	b, err := remote.Open(cfg)
	if err != nil {
		return nil, err
	}
	info, err := b.Info()
	if err != nil {
		return nil, err
	}
	if _, err := b.Projects(); err != nil {
		return nil, err
	}
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Upsert(cfg, info.Name)
	id := t.ID
	if err := store.Save(); err != nil {
		return nil, err
	}
	return store.Find(id), nil
}

// PollInterval is how often the agent should check the backend.
func (r *Repo) PollInterval() time.Duration {
	if r.Config.Remote == nil {
		return 5 * time.Second
	}
	return r.Config.Remote.PollInterval()
}

// --- history ---

// ancestors returns id and every snapshot reachable through parents.
func (r *Repo) ancestors(id string) (map[string]bool, error) {
	seen := map[string]bool{}
	stack := []string{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == "" || seen[cur] {
			continue
		}
		seen[cur] = true
		m, err := r.Load(cur)
		if err != nil {
			return nil, err
		}
		stack = append(stack, m.Parents...)
	}
	return seen, nil
}

// isAncestor reports whether a is b or one of b's ancestors.
func (r *Repo) isAncestor(a, b string) (bool, error) {
	anc, err := r.ancestors(b)
	return anc[a], err
}

// mergeBase finds the nearest common ancestor of a and b.
func (r *Repo) mergeBase(a, b string) (string, error) {
	anc, err := r.ancestors(a)
	if err != nil {
		return "", err
	}
	queue, seen := []string{b}, map[string]bool{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		if anc[cur] {
			return cur, nil
		}
		m, err := r.Load(cur)
		if err != nil {
			return "", err
		}
		queue = append(queue, m.Parents...)
	}
	return "", nil
}

// --- transfer ---

// fetchSnapshots downloads id and any ancestors not stored locally.
func (r *Repo) fetchSnapshots(c remote.Backend, id string) error {
	stack := []string{id}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == "" || r.HasSnapshot(cur) {
			continue
		}
		data, err := c.GetSnapshot(r.Config.ProjectID, cur)
		if err != nil {
			return fmt.Errorf("download version %s: %w", short(cur), err)
		}
		if err := r.storeSnapshot(cur, data); err != nil {
			return err
		}
		m, _ := r.Load(cur)
		stack = append(stack, m.Parents...)
	}
	return nil
}

// fetchObjects downloads blobs that are not stored locally.
func (r *Repo) fetchObjects(c remote.Backend, hashes []string) error {
	var need []string
	var total int64
	for _, h := range hashes {
		if !r.Store.Has(h) {
			need = append(need, h)
			total += r.sizes[h] // 0 when not known
		}
	}
	t := r.newTransfer(StageDownloading, len(need), total)
	for i, h := range need {
		t.done = i
		t.report()
		body, err := c.GetObject(h)
		if err != nil {
			return fmt.Errorf("download %s: %w", short(h), err)
		}
		got, _, err := r.Store.Put(t.reader(body, -1))
		body.Close()
		if err != nil {
			return err
		}
		if got != h {
			return fmt.Errorf("download %s: content hash mismatch", short(h))
		}
	}
	return nil
}

// publish uploads everything HEAD needs and moves the current branch from
// old to HEAD.
func (r *Repo) publish(c remote.Backend, old string) error {
	return r.publishTo(c, r.BranchName(), old)
}

// publishTo uploads everything HEAD needs and moves branch from old to HEAD.
func (r *Repo) publishTo(c remote.Backend, branch, old string) error {
	head := r.Head()
	if err := c.PutProject(remote.Project{ID: r.Config.ProjectID, Name: r.Config.Name}); err != nil {
		return err
	}
	// Snapshots in parent-first order.
	var order []string
	seen := map[string]bool{}
	var visit func(id string) error
	visit = func(id string) error {
		if id == "" || seen[id] {
			return nil
		}
		seen[id] = true
		m, err := r.Load(id)
		if err != nil {
			return err
		}
		for _, p := range m.Parents {
			if err := visit(p); err != nil {
				return err
			}
		}
		order = append(order, id)
		return nil
	}
	if err := visit(head); err != nil {
		return err
	}
	missing, err := c.MissingSnapshots(r.Config.ProjectID, order)
	if err != nil {
		return err
	}
	need := map[string]bool{}
	for _, id := range missing {
		need[id] = true
	}
	for _, id := range order {
		if !need[id] {
			continue
		}
		m, err := r.Load(id)
		if err != nil {
			return err
		}
		if err := r.uploadObjects(c, m.Objects()); err != nil {
			return err
		}
		data, err := os.ReadFile(r.snapshotPath(id))
		if err != nil {
			return err
		}
		if err := c.PutSnapshot(r.Config.ProjectID, id, data); err != nil {
			return err
		}
	}
	return c.UpdateBranch(r.Config.ProjectID, branch, old, head)
}

func (r *Repo) uploadObjects(c remote.Backend, hashes []string) error {
	missing, err := c.MissingObjects(dedupe(hashes))
	if err != nil {
		return err
	}
	sizes := map[string]int64{}
	var total int64
	for _, h := range missing {
		if fi, err := os.Stat(r.Store.Path(h)); err == nil {
			sizes[h] = fi.Size()
			total += fi.Size()
		}
	}
	t := r.newTransfer(StageUploading, len(missing), total)
	for i, h := range missing {
		t.done = i
		t.report()
		f, err := r.Store.Open(h)
		if err != nil {
			return err
		}
		size, ok := sizes[h]
		if !ok {
			size = -1
		}
		err = c.PutObject(h, t.reader(f, size))
		f.Close()
		if err != nil {
			return fmt.Errorf("upload %s: %w", short(h), err)
		}
	}
	return nil
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func short(id string) string { return id[:min(10, len(id))] }

// Incoming reports whether the server branch has versions this workspace does
// not have yet (taking them rewrites working files).
func (r *Repo) Incoming() (bool, error) {
	c, err := r.Client()
	if err != nil {
		return false, err
	}
	branches, err := c.Branches(r.Config.ProjectID)
	if errors.Is(err, remote.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	remoteHead := branches[r.BranchName()]
	if remoteHead == "" || remoteHead == r.Latest() {
		return false, nil
	}
	if err := r.fetchSnapshots(c, remoteHead); err != nil {
		return false, err
	}
	have, err := r.isAncestor(remoteHead, r.Latest())
	return !have, err
}

// --- update / save ---

type SyncResult struct {
	// Action is "up-to-date", "ahead", "fast-forward", "merged" or "published".
	Action   string
	From, To string
	// MergeLog describes what a merge took from each side.
	MergeLog []string
	// Relinked lists sample paths rewritten for this machine.
	Relinked []string
}

// Update brings the workspace up to date with the server branch: a fast
// forward when only the server moved, a merge when both did. The working
// files must match HEAD.
func (r *Repo) Update(opts MergeOptions) (*SyncResult, error) {
	if err := r.guardLatest(); err != nil {
		return nil, err
	}
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	branches, err := c.Branches(r.Config.ProjectID)
	if errors.Is(err, remote.ErrNotFound) {
		return &SyncResult{Action: "up-to-date"}, nil // project not published yet
	}
	if err != nil {
		return nil, err
	}
	return r.integrate(c, branches[r.BranchName()], opts, "Merge versions from the team")
}

// integrate brings version target (and its history) into the workspace: a
// fast forward when HEAD is behind, otherwise a merge version with message.
func (r *Repo) integrate(c remote.Backend, target string, opts MergeOptions, message string) (*SyncResult, error) {
	head := r.Head()
	res := &SyncResult{From: head, To: head}
	if target == "" || target == head {
		res.Action = "up-to-date"
		return res, nil
	}
	if err := r.fetchSnapshots(c, target); err != nil {
		return nil, err
	}
	if head != "" {
		if ahead, err := r.isAncestor(target, head); err != nil || ahead {
			res.Action = "ahead"
			return res, err
		}
		// Integrating rewrites working files: refuse before doing any work.
		changes, err := r.Status()
		if err != nil {
			return nil, err
		}
		if len(changes) > 0 {
			return nil, fmt.Errorf("%w (%d file(s)); save a version first", ErrDirty, len(changes))
		}
	}
	res.Action = "fast-forward"
	if head != "" {
		behind, err := r.isAncestor(head, target)
		if err != nil {
			return nil, err
		}
		if !behind {
			merged, log, err := r.mergeWith(c, head, target, opts, message)
			if err != nil {
				return nil, err
			}
			target, res.Action, res.MergeLog = merged.ID, "merged", log
		}
	}
	m, err := r.Load(target)
	if err != nil {
		return nil, err
	}
	r.knowSizes(m)
	if err := r.fetchObjects(c, m.Objects()); err != nil {
		return nil, err
	}
	_, notes, err := r.Checkout(target, head == "")
	if err != nil {
		return nil, err
	}
	res.To, res.Relinked = target, notes
	return res, nil
}

// Save records the working files as a version and shares it: versions saved
// by others in the meantime are merged in first.
func (r *Repo) Save(message string, opts MergeOptions) (*Manifest, *SyncResult, error) {
	m, err := r.Snapshot(message)
	if err != nil && !errors.Is(err, ErrNothingToSnapshot) {
		return nil, nil, err
	}
	c, err := r.Client()
	if err != nil {
		return m, nil, err
	}
	res := &SyncResult{}
	for attempt := 0; attempt < 5; attempt++ {
		branches, err := c.Branches(r.Config.ProjectID)
		if err != nil && !errors.Is(err, remote.ErrNotFound) {
			return m, nil, err
		}
		remoteHead := branches[r.BranchName()]
		if remoteHead == r.Head() {
			if res.Action == "" {
				res.Action = "up-to-date"
			}
			return m, res, nil
		}
		ahead := remoteHead == ""
		if !ahead {
			if err := r.fetchSnapshots(c, remoteHead); err != nil {
				return m, nil, err
			}
			if ahead, err = r.isAncestor(remoteHead, r.Head()); err != nil {
				return m, nil, err
			}
		}
		if !ahead {
			up, err := r.Update(opts)
			if err != nil {
				return m, nil, err
			}
			res.MergeLog = append(res.MergeLog, up.MergeLog...)
			res.Relinked = append(res.Relinked, up.Relinked...)
			if up.Action == "fast-forward" {
				// Nothing of ours to share (e.g. an empty save).
				res.Action = "fast-forward"
				return m, res, nil
			}
			continue // publish the merge
		}
		err = r.publish(c, remoteHead)
		var conflict *remote.ErrConflict
		if errors.As(err, &conflict) {
			continue // someone saved at the same moment; merge and retry
		}
		if err != nil {
			return m, nil, err
		}
		res.Action, res.To = "published", r.Head()
		return m, res, nil
	}
	return m, nil, errors.New("the server branch keeps changing; try again")
}

// Clone connects to a team (address + token, or a connection code) and
// downloads one of its projects into dir.
func Clone(address, token, project, dir, author string) (*Repo, *Manifest, error) {
	t, err := Connect(address, token)
	if err != nil {
		return nil, nil, err
	}
	return CloneFromTeam(t, project, dir, author, nil)
}

// CloneFromTeam downloads a project (name or id) of a connected team into
// dir (default "<name> Project") and records where it is. onProgress may be
// nil.
func CloneFromTeam(t *teams.Team, project, dir, author string, onProgress func(Progress)) (*Repo, *Manifest, error) {
	cfg := t.Remote
	c, err := remote.Open(cfg)
	if err != nil {
		return nil, nil, err
	}
	projects, err := c.Projects()
	if err != nil {
		return nil, nil, err
	}
	var match []remote.Project
	for _, p := range projects {
		if p.ID == project || strings.EqualFold(p.Name, project) || strings.HasPrefix(p.ID, project) {
			match = append(match, p)
		}
	}
	switch {
	case len(match) == 0:
		return nil, nil, fmt.Errorf("no project %q on %s", project, cfg.Display())
	case len(match) > 1:
		return nil, nil, fmt.Errorf("%q matches %d projects; use the project id", project, len(match))
	}
	p := match[0]
	if dir == "" {
		dir = p.Name + " Project"
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, nil, err
	}
	if entries, err := os.ReadDir(root); err == nil && len(entries) > 0 {
		return nil, nil, fmt.Errorf("%s is not empty", root)
	}
	if author == "" {
		author = defaultAuthor()
	}
	r, err := create(root, Config{ProjectID: p.ID, Name: p.Name, Author: author,
		Remote: &RemoteConfig{URL: cfg.URL}})
	if err != nil {
		return nil, nil, err
	}
	r.OnProgress = onProgress
	if err := r.JoinTeam(t); err != nil {
		return nil, nil, err
	}
	res, err := r.Update(Strategy("fail"))
	if err != nil {
		return r, nil, err
	}
	var m *Manifest
	if res.To != "" {
		m, err = r.Load(res.To)
	}
	return r, m, err
}

// --- snapshot merge ---

// MergeConflictError lists what could not be merged automatically.
type MergeConflictError struct{ Conflicts []ConflictItem }

func (e *MergeConflictError) Error() string {
	lines := make([]string, len(e.Conflicts))
	for i, c := range e.Conflicts {
		lines[i] = c.String()
	}
	return fmt.Sprintf("%d conflict(s) need a decision:\n  %s", len(e.Conflicts), strings.Join(lines, "\n  "))
}

// mergeWith creates a merge snapshot of ours and theirs (not checked out).
func (r *Repo) mergeWith(c remote.Backend, ours, theirs string, opts MergeOptions, message string) (*Manifest, []string, error) {
	baseID, err := r.mergeBase(ours, theirs)
	if err != nil {
		return nil, nil, err
	}
	o, err := r.Load(ours)
	if err != nil {
		return nil, nil, err
	}
	t, err := r.Load(theirs)
	if err != nil {
		return nil, nil, err
	}
	b := &Manifest{}
	if baseID != "" {
		if b, err = r.Load(baseID); err != nil {
			return nil, nil, err
		}
	}
	// Merging needs theirs' blobs and base's sets.
	need := t.Objects()
	for _, f := range b.Files {
		if isSet(f.Path) {
			need = append(need, f.Hash)
		}
	}
	r.knowSizes(t, b)
	if err := r.fetchObjects(c, need); err != nil {
		return nil, nil, err
	}
	m, log, err := r.mergeManifests(b, o, t, opts)
	if err != nil {
		return nil, nil, err
	}
	m.Message = message
	if err := r.save(m); err != nil {
		return nil, nil, err
	}
	return m, log, nil
}

// mergeManifests 3-way merges file lists; Live Sets changed on both sides are
// merged track by track.
func (r *Repo) mergeManifests(base, ours, theirs *Manifest, opts MergeOptions) (*Manifest, []string, error) {
	bf, of, tf := base.FileMap(), ours.FileMap(), theirs.FileMap()
	paths := map[string]bool{}
	for _, m := range []map[string]FileEntry{bf, of, tf} {
		for p := range m {
			paths[p] = true
		}
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	var files []FileEntry
	var log []string
	var conflicts []ConflictItem
	for _, p := range sorted {
		b, o, t := bf[p], of[p], tf[p]
		switch {
		case o.Hash == t.Hash, t.Hash == b.Hash:
			if o.Hash != "" {
				files = append(files, o)
			}
			continue
		case o.Hash == b.Hash:
			if t.Hash != "" {
				files = append(files, t)
				log = append(log, "took theirs: "+p)
			} else {
				log = append(log, "deleted (theirs): "+p)
			}
			continue
		}
		// Changed on both sides.
		if isSet(p) && b.Hash != "" && o.Hash != "" && t.Hash != "" {
			entry, setLog, setConflicts, err := r.mergeSet(p, b.Hash, o.Hash, t.Hash, opts)
			if err != nil {
				return nil, nil, err
			}
			log = append(log, setLog...)
			conflicts = append(conflicts, setConflicts...)
			files = append(files, entry)
			continue
		}
		what := "changed on both sides"
		switch {
		case o.Hash == "":
			what = "deleted by you, changed by others"
		case t.Hash == "":
			what = "changed by you, deleted by others"
		}
		key := "file:" + p
		switch opts.choice(key) {
		case "theirs":
			if t.Hash != "" {
				files = append(files, t)
			}
			log = append(log, p+": "+what+" -> took theirs")
		case "both":
			if o.Hash != "" {
				files = append(files, o)
			}
			if t.Hash != "" {
				alt := t
				alt.Path = theirsName(p, paths)
				files = append(files, alt)
				log = append(log, p+": "+what+" -> kept both (theirs as "+alt.Path+")")
			}
		case "ours":
			if o.Hash != "" {
				files = append(files, o)
			}
			log = append(log, p+": "+what+" -> kept yours")
		default:
			conflicts = append(conflicts, ConflictItem{Key: key, File: p, Unit: p, Description: what,
				CanKeepBoth: o.Hash != "" && t.Hash != ""})
		}
	}
	if len(conflicts) > 0 {
		return nil, nil, &MergeConflictError{Conflicts: conflicts}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	authorID, author := r.Identity()
	m := &Manifest{Version: 1, Parents: []string{ours.ID, theirs.ID}, Author: author, AuthorID: authorID,
		Time: time.Now().UTC().Format(time.RFC3339), Message: "Merge versions from the team",
		Files: files}
	ext := map[string]FileEntry{}
	for _, e := range append(append([]FileEntry{}, theirs.External...), ours.External...) {
		ext[e.Path] = e // ours wins
	}
	for _, e := range ext {
		m.External = append(m.External, e)
	}
	sort.Slice(m.External, func(i, j int) bool { return m.External[i].Path < m.External[j].Path })
	m.Packs = union(ours.Packs, theirs.Packs)
	m.Missing = union(ours.Missing, theirs.Missing)
	return m, log, nil
}

func union(a, b []string) []string {
	out := dedupe(append(append([]string{}, a...), b...))
	sort.Strings(out)
	return out
}

// theirsName picks "<name> (theirs)<ext>" not colliding with existing paths.
func theirsName(p string, taken map[string]bool) string {
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	name := stem + " (theirs)" + ext
	for i := 2; taken[name]; i++ {
		name = fmt.Sprintf("%s (theirs %d)%s", stem, i, ext)
	}
	taken[name] = true
	return name
}
