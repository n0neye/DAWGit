package project

import (
	"errors"
	"fmt"
	"sort"

	"dawgit/internal/als"
	"dawgit/internal/diff"
	"dawgit/internal/remote"
)

// BranchInfo is a server branch and its latest version.
type BranchInfo struct {
	Name    string
	Head    string
	Current bool
	Latest  *Manifest // nil if not downloaded
}

// Branches lists the server's branches, the current one marked.
func (r *Repo) Branches() ([]BranchInfo, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if err != nil && !errors.Is(err, remote.ErrNotFound) {
		return nil, err
	}
	var out []BranchInfo
	for name, head := range heads {
		b := BranchInfo{Name: name, Head: head, Current: name == r.BranchName()}
		if r.fetchSnapshots(c, head) == nil {
			b.Latest, _ = r.Load(head)
		}
		out = append(out, b)
	}
	if _, ok := heads[r.BranchName()]; !ok {
		out = append(out, BranchInfo{Name: r.BranchName(), Current: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ErrUnshared means the workspace has versions the current branch lacks.
var ErrUnshared = errors.New("you have versions that are not shared yet; run `dawgit save` first")

// shared reports whether HEAD is already on the current server branch.
func (r *Repo) shared(c *remote.Client) (bool, error) {
	heads, err := c.Branches(r.Config.ProjectID)
	if errors.Is(err, remote.ErrNotFound) {
		return r.Head() == "", nil
	}
	if err != nil {
		return false, err
	}
	head, remoteHead := r.Head(), heads[r.BranchName()]
	if head == "" || head == remoteHead {
		return true, nil
	}
	if remoteHead == "" {
		return false, nil
	}
	if err := r.fetchSnapshots(c, remoteHead); err != nil {
		return false, err
	}
	return r.isAncestor(head, remoteHead)
}

// CreateBranch starts a branch at the current version and switches to it.
func (r *Repo) CreateBranch(name string) error {
	c, err := r.Client()
	if err != nil {
		return err
	}
	if r.Head() == "" {
		return errors.New("save a first version before creating branches")
	}
	if err := r.publishTo(c, name, ""); err != nil {
		var conflict *remote.ErrConflict
		if errors.As(err, &conflict) {
			return fmt.Errorf("branch %q already exists", name)
		}
		return err
	}
	r.Config.Branch = name
	return r.SaveConfig()
}

// SwitchBranch makes the workspace follow another branch and checks out its
// latest version. Unsaved changes and unshared versions block the switch
// unless force is set.
func (r *Repo) SwitchBranch(name string, force bool) (*SyncResult, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if err != nil {
		return nil, err
	}
	target, ok := heads[name]
	if !ok {
		return nil, fmt.Errorf("no branch %q on the server (create it with `dawgit branch new %s`)", name, name)
	}
	if !force {
		if ok, err := r.shared(c); err != nil {
			return nil, err
		} else if !ok {
			return nil, ErrUnshared
		}
	}
	if err := r.fetchSnapshots(c, target); err != nil {
		return nil, err
	}
	m, err := r.Load(target)
	if err != nil {
		return nil, err
	}
	if err := r.fetchObjects(c, m.Objects()); err != nil {
		return nil, err
	}
	from := r.Head()
	_, notes, err := r.Checkout(target, force)
	if err != nil {
		return nil, err
	}
	r.Config.Branch = name
	if err := r.SaveConfig(); err != nil {
		return nil, err
	}
	return &SyncResult{Action: "switched", From: from, To: target, Relinked: notes}, nil
}

// MergeBranch merges another branch's latest version into the workspace and
// shares the result on the current branch.
func (r *Repo) MergeBranch(name string, opts MergeOptions) (*SyncResult, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if err != nil {
		return nil, err
	}
	target, ok := heads[name]
	if !ok {
		return nil, fmt.Errorf("no branch %q on the server", name)
	}
	if name == r.BranchName() {
		return nil, errors.New("that is the branch you are on; use `dawgit update`")
	}
	res, err := r.integrate(c, target, opts, "Merge branch "+name)
	if err != nil || res.Action == "up-to-date" || res.Action == "ahead" {
		return res, err
	}
	_, shared, err := r.Save("", opts)
	if err != nil {
		return res, err
	}
	res.MergeLog = append(res.MergeLog, shared.MergeLog...)
	res.Relinked = append(res.Relinked, shared.Relinked...)
	res.To = r.Head()
	return res, nil
}

// --- preview ---

type FileChange struct {
	Path   string
	Status string // "added" | "modified" | "deleted"
	// SetDiff is the semantic diff of a modified Live Set.
	SetDiff *diff.SetDiff
}

// Preview describes what integrating a version would bring in.
type Preview struct {
	// Action is "up-to-date", "ahead", "fast-forward" or "merge".
	Action   string
	Versions []*Manifest // incoming versions, newest first
	Changes  []FileChange
	// Conflicts that need a decision.
	Conflicts []ConflictItem
}

// PreviewUpdate previews `update` on the current branch.
func (r *Repo) PreviewUpdate() (*Preview, error) {
	return r.previewBranch(r.BranchName())
}

// PreviewMerge previews merging another branch.
func (r *Repo) PreviewMerge(name string) (*Preview, error) {
	return r.previewBranch(name)
}

func (r *Repo) previewBranch(name string) (*Preview, error) {
	c, err := r.Client()
	if err != nil {
		return nil, err
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if err != nil && !errors.Is(err, remote.ErrNotFound) {
		return nil, err
	}
	target, ok := heads[name]
	if !ok && name != r.BranchName() {
		return nil, fmt.Errorf("no branch %q on the server", name)
	}
	head := r.Head()
	p := &Preview{Action: "up-to-date"}
	if target == "" || target == head {
		return p, nil
	}
	if err := r.fetchSnapshots(c, target); err != nil {
		return nil, err
	}
	if ahead, err := r.isAncestor(target, head); err != nil || ahead {
		p.Action = "ahead"
		return p, err
	}
	baseID, err := r.mergeBase(head, target)
	if err != nil {
		return nil, err
	}
	p.Action = "merge"
	if baseID == head {
		p.Action = "fast-forward"
	}

	// Incoming versions: reachable from target but not from HEAD.
	have, err := r.ancestors(head)
	if err != nil {
		return nil, err
	}
	incoming, err := r.ancestors(target)
	if err != nil {
		return nil, err
	}
	for id := range incoming {
		if !have[id] {
			m, _ := r.Load(id)
			p.Versions = append(p.Versions, m)
		}
	}
	sort.Slice(p.Versions, func(i, j int) bool { return p.Versions[i].Time > p.Versions[j].Time })

	base := &Manifest{}
	if baseID != "" {
		if base, err = r.Load(baseID); err != nil {
			return nil, err
		}
	}
	theirs, _ := r.Load(target)
	need := theirs.Objects()
	for _, f := range base.Files {
		if isSet(f.Path) {
			need = append(need, f.Hash)
		}
	}
	if err := r.fetchObjects(c, need); err != nil {
		return nil, err
	}
	p.Changes, err = r.fileChanges(base, theirs)
	if err != nil {
		return nil, err
	}

	if p.Action == "merge" {
		ours, err := r.Load(head)
		if err != nil {
			return nil, err
		}
		_, _, err = r.mergeManifests(base, ours, theirs, Strategy("fail"))
		var conflict *MergeConflictError
		if errors.As(err, &conflict) {
			p.Conflicts = conflict.Conflicts
		} else if err != nil {
			return nil, err
		}
	}
	return p, nil
}

// fileChanges lists what changed from a to b, with semantic diffs for sets.
func (r *Repo) fileChanges(a, b *Manifest) ([]FileChange, error) {
	af, bf := a.FileMap(), b.FileMap()
	var out []FileChange
	for _, f := range b.Files {
		old, ok := af[f.Path]
		switch {
		case !ok:
			out = append(out, FileChange{Path: f.Path, Status: "added"})
		case old.Hash != f.Hash:
			fc := FileChange{Path: f.Path, Status: "modified"}
			if isSet(f.Path) {
				fc.SetDiff = r.storedSetDiff(old.Hash, f.Hash)
			}
			out = append(out, fc)
		}
	}
	for _, f := range a.Files {
		if _, ok := bf[f.Path]; !ok {
			out = append(out, FileChange{Path: f.Path, Status: "deleted"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (r *Repo) storedSetDiff(oldHash, newHash string) *diff.SetDiff {
	load := func(h string) *als.LiveSet {
		data, err := r.Store.Read(h)
		if err != nil {
			return nil
		}
		s, _ := als.FromGzip(data)
		return s
	}
	a, b := load(oldHash), load(newHash)
	if a == nil || b == nil {
		return nil
	}
	return diff.Diff(a, b)
}
