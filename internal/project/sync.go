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
)

// ErrNoRemote is returned by sync operations without a configured server.
var ErrNoRemote = errors.New("no server configured (run `dawgit remote <url>`)")

func (r *Repo) Client() (*remote.Client, error) {
	if r.Config.Remote == nil || r.Config.Remote.URL == "" {
		return nil, ErrNoRemote
	}
	return remote.New(r.Config.Remote.URL, r.Config.Remote.Token), nil
}

func (r *Repo) SetRemote(url, token string) error {
	r.Config.Remote = &RemoteConfig{URL: strings.TrimRight(url, "/"), Token: token}
	return r.SaveConfig()
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
func (r *Repo) fetchSnapshots(c *remote.Client, id string) error {
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
func (r *Repo) fetchObjects(c *remote.Client, hashes []string) error {
	for _, h := range hashes {
		if r.Store.Has(h) {
			continue
		}
		body, err := c.GetObject(h)
		if err != nil {
			return fmt.Errorf("download %s: %w", short(h), err)
		}
		got, _, err := r.Store.Put(body)
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

// publish uploads everything HEAD needs and moves the branch from old to HEAD.
func (r *Repo) publish(c *remote.Client, old string) error {
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
	return c.UpdateBranch(r.Config.ProjectID, r.BranchName(), old, head)
}

func (r *Repo) uploadObjects(c *remote.Client, hashes []string) error {
	missing, err := c.MissingObjects(dedupe(hashes))
	if err != nil {
		return err
	}
	for _, h := range missing {
		f, err := r.Store.Open(h)
		if err != nil {
			return err
		}
		err = c.PutObject(h, f)
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
	if remoteHead == "" || remoteHead == r.Head() {
		return false, nil
	}
	if err := r.fetchSnapshots(c, remoteHead); err != nil {
		return false, err
	}
	have, err := r.isAncestor(remoteHead, r.Head())
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
func (r *Repo) Update(strategy string) (*SyncResult, error) {
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
	head, remoteHead := r.Head(), branches[r.BranchName()]
	res := &SyncResult{From: head, To: head}
	if remoteHead == "" || remoteHead == head {
		res.Action = "up-to-date"
		return res, nil
	}
	if err := r.fetchSnapshots(c, remoteHead); err != nil {
		return nil, err
	}
	if head != "" {
		if ahead, err := r.isAncestor(remoteHead, head); err != nil || ahead {
			res.Action = "ahead"
			return res, err
		}
		// Updating rewrites working files: refuse before doing any work.
		changes, err := r.Status()
		if err != nil {
			return nil, err
		}
		if len(changes) > 0 {
			return nil, fmt.Errorf("%w (%d file(s)); save a version first", ErrDirty, len(changes))
		}
	}
	target := remoteHead
	res.Action = "fast-forward"
	if head != "" {
		behind, err := r.isAncestor(head, remoteHead)
		if err != nil {
			return nil, err
		}
		if !behind {
			merged, log, err := r.mergeWith(c, head, remoteHead, strategy)
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
func (r *Repo) Save(message, strategy string) (*Manifest, *SyncResult, error) {
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
			up, err := r.Update(strategy)
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

// Clone downloads a project from a server into dir.
func Clone(url, token, project, dir, author string) (*Repo, *Manifest, error) {
	c := remote.New(url, token)
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
		return nil, nil, fmt.Errorf("no project %q on %s", project, url)
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
		Remote: &RemoteConfig{URL: c.URL, Token: token}})
	if err != nil {
		return nil, nil, err
	}
	res, err := r.Update("fail")
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
type MergeConflictError struct{ Conflicts []string }

func (e *MergeConflictError) Error() string {
	return fmt.Sprintf("%d conflict(s) need a decision:\n  %s", len(e.Conflicts), strings.Join(e.Conflicts, "\n  "))
}

// mergeWith creates a merge snapshot of ours and theirs (not checked out).
func (r *Repo) mergeWith(c *remote.Client, ours, theirs, strategy string) (*Manifest, []string, error) {
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
	if err := r.fetchObjects(c, need); err != nil {
		return nil, nil, err
	}
	m, log, err := r.mergeManifests(b, o, t, strategy)
	if err != nil {
		return nil, nil, err
	}
	if err := r.save(m); err != nil {
		return nil, nil, err
	}
	return m, log, nil
}

// mergeManifests 3-way merges file lists; Live Sets changed on both sides are
// merged track by track.
func (r *Repo) mergeManifests(base, ours, theirs *Manifest, strategy string) (*Manifest, []string, error) {
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
	var log, conflicts []string
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
			entry, setLog, setConflicts, err := r.mergeSet(p, b.Hash, o.Hash, t.Hash, strategy)
			if err != nil {
				return nil, nil, err
			}
			log = append(log, setLog...)
			conflicts = append(conflicts, setConflicts...)
			files = append(files, entry)
			continue
		}
		what := p + ": changed on both sides"
		switch {
		case o.Hash == "":
			what = p + ": deleted by you, changed by others"
		case t.Hash == "":
			what = p + ": changed by you, deleted by others"
		}
		switch strategy {
		case "theirs":
			if t.Hash != "" {
				files = append(files, t)
			}
			log = append(log, what+" -> took theirs")
		case "both":
			if o.Hash != "" {
				files = append(files, o)
			}
			if t.Hash != "" {
				alt := t
				alt.Path = theirsName(p, paths)
				files = append(files, alt)
				log = append(log, what+" -> kept both (theirs as "+alt.Path+")")
			}
		case "ours":
			if o.Hash != "" {
				files = append(files, o)
			}
			log = append(log, what+" -> kept yours")
		default:
			conflicts = append(conflicts, what)
		}
	}
	if len(conflicts) > 0 {
		return nil, nil, &MergeConflictError{Conflicts: conflicts}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	m := &Manifest{Version: 1, Parents: []string{ours.ID, theirs.ID}, Author: r.Config.Author,
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
