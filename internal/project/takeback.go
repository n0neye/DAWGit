package project

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"dawgit/internal/remote"
)

// Taking back the latest version (an undo that leaves no trace, as in
// Diversion): the team's branch goes back to the version before it, and its
// changes come back here as uncommitted changes, to fix and commit again.
// The version is gone from the history; the branch log keeps the move.
//
// Only your own latest version, and only while no one else has it. Every
// copy of a project says in the team's storage which of the team's versions
// it has taken in (its workspace record, written before its files change),
// and other branches must not have it. A copy that took it in all the same
// (at that very moment) finds the team's branch went back from the version
// it saw last, and takes the version out of its files, keeping its own work
// (dropTakenBack).
//
// The records are something a team stores: taking back is a team feature,
// so a DAWGit that writes none (and could share the version again) stops
// before working with the team.

// FeatureTakeBack is the team feature for taking versions back.
const FeatureTakeBack = "take-back"

func init() { remote.RegisterFeature(FeatureTakeBack) }

// workspaceRecord is what a copy of a project tells the team about itself.
type workspaceRecord struct {
	ID     string    `json:"id"`
	Member string    `json:"member,omitempty"`
	Branch string    `json:"branch"`
	Has    string    `json:"has"` // the team's latest version of Branch it took in
	Time   time.Time `json:"time"`
}

// seenFile maps each branch to the team's latest version this copy took in.
const seenFile = "team-seen.json"

func (r *Repo) seenHeads() map[string]string {
	seen := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(r.Dir, seenFile)); err == nil {
		json.Unmarshal(data, &seen)
	}
	return seen
}

// noteTeamHead records that this copy takes in id, the team's latest
// version of its branch: here, and in the team's storage. Called before the
// files change; as well as can be (a record that didn't go up doesn't stop
// an update: the copy finds out later if the version is taken back).
func (r *Repo) noteTeamHead(c remote.Backend, id string) {
	branch := r.BranchName()
	seen := r.seenHeads()
	if id == "" || seen[branch] == id {
		return
	}
	seen[branch] = id
	data, _ := json.Marshal(seen)
	os.WriteFile(filepath.Join(r.Dir, seenFile), data, 0o644)
	if c == nil {
		return
	}
	if r.Config.WorkspaceID == "" {
		b := make([]byte, 16)
		rand.Read(b)
		r.Config.WorkspaceID = hex.EncodeToString(b)
		if r.SaveConfig() != nil {
			return
		}
	}
	member, _ := r.Identity()
	c.PutWorkspace(r.Config.ProjectID, r.Config.WorkspaceID, workspaceRecord{
		ID: r.Config.WorkspaceID, Member: member, Branch: branch, Has: id, Time: time.Now().UTC()})
}

// TakeBackPlan says whether a version can be taken back.
type TakeBackPlan struct {
	OK bool
	// Why not: "older-version", "not-latest" (versions came after it),
	// "not-yours", "who" (can't tell whose it is), "merge", "first", "has-it" (HaveIt), "on-branch"
	// (Branches); "" when OK.
	Why      string
	HaveIt   []string // member ids of the copies that have it ("" unknown)
	Branches []string // other branches that have it
	// Shared: the team has it (else it is only here). FeatureOff: the team
	// must turn taking back on first (TakeBack with turnOn).
	Shared, FeatureOff bool
	parent             string
}

// PlanTakeBack says whether version id can be taken back.
func (r *Repo) PlanTakeBack(id string) (*TakeBackPlan, error) {
	p := &TakeBackPlan{}
	if r.OnOlderVersion() {
		p.Why = "older-version"
		return p, nil
	}
	if id != r.Head() {
		p.Why = "not-latest"
		return p, nil
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, err
	}
	switch len(m.Parents) {
	case 0:
		p.Why = "first"
		return p, nil
	case 1:
		p.parent = m.Parents[0]
	default:
		p.Why = "merge"
		return p, nil
	}
	c, err := r.Client()
	if errors.Is(err, ErrNoRemote) {
		p.OK = true
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	if me, _ := r.Identity(); me == "" || m.AuthorID == "" {
		p.Why = "who" // no one to tell it by: a name not chosen, or an older version
		return p, nil
	} else if m.AuthorID != me {
		p.Why = "not-yours"
		return p, nil
	}
	pid := r.Config.ProjectID
	heads, err := c.Branches(pid)
	if errors.Is(err, remote.ErrNotFound) {
		heads, err = map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	has := func(v string) (bool, error) {
		if v == "" {
			return false, nil
		}
		if err := r.fetchSnapshots(c, v); err != nil {
			return false, err
		}
		return r.isAncestor(id, v)
	}
	branch := r.BranchName()
	for name, h := range heads {
		in, err := has(h)
		if err != nil {
			return nil, err
		}
		switch {
		case !in:
		case name != branch:
			p.Branches = append(p.Branches, name)
		case h != id:
			p.Why = "not-latest" // the team's versions came after it
			return p, nil
		default:
			p.Shared = true
		}
	}
	if len(p.Branches) > 0 {
		sort.Strings(p.Branches)
		p.Why = "on-branch"
		return p, nil
	}
	if !p.Shared {
		p.OK = true
		return p, nil
	}
	var recs []workspaceRecord
	if err := c.Workspaces(pid, &recs); err != nil {
		return nil, err
	}
	for _, rec := range recs {
		if rec.ID == r.Config.WorkspaceID || rec.Has == "" || rec.Has == p.parent {
			continue
		}
		in, err := has(rec.Has)
		if err != nil || in { // a version that can't be read: it may have it
			if !slices.Contains(p.HaveIt, rec.Member) {
				p.HaveIt = append(p.HaveIt, rec.Member)
			}
		}
	}
	if len(p.HaveIt) > 0 {
		p.Why = "has-it"
		return p, nil
	}
	info, err := c.Info()
	if err != nil {
		return nil, err
	}
	p.FeatureOff = !slices.Contains(info.Features, FeatureTakeBack)
	p.OK = true
	return p, nil
}

// ErrTakeBackOff: the team hasn't turned taking back on (FeatureTakeBack).
var ErrTakeBackOff = errors.New("taking back versions isn't turned on for this team: " +
	"turning it on asks teammates with an older DAWGit to update")

// TakeBack takes back the latest version id: the project (and the team's
// branch, when it was shared) goes back to the version before it; its
// changes stay in the files, uncommitted. turnOn turns taking back on for
// the team when it isn't (see FeatureTakeBack).
func (r *Repo) TakeBack(id string, turnOn bool) (*TakeBackPlan, error) {
	p, err := r.PlanTakeBack(id)
	if err != nil {
		return nil, err
	}
	if !p.OK {
		return p, fmt.Errorf("that version can't be taken back (%s)", p.Why)
	}
	var c remote.Backend
	if p.Shared {
		if c, err = r.Client(); err != nil {
			return nil, err
		}
		if p.FeatureOff {
			if !turnOn {
				return p, ErrTakeBackOff
			}
			if err := remote.EnableFeature(c, FeatureTakeBack); err != nil {
				return nil, err
			}
		}
		err := c.UpdateBranch(r.Config.ProjectID, r.BranchName(), id, p.parent)
		var conflict *remote.ErrConflict
		if errors.As(err, &conflict) {
			return nil, errors.New("someone shared a version just now: take it in, then undo with a new version")
		}
		if err != nil {
			return nil, err
		}
	}
	if err := r.setHead(p.parent); err != nil {
		return nil, err
	}
	if p.Shared {
		r.noteTeamHead(c, p.parent)
	}
	return p, nil
}

// TakenBackFrom lists the versions this copy has that the team took back
// (newest first), from branch heads already fetched (see FetchTeam): no
// network.
func (r *Repo) TakenBackFrom(heads map[string]string) ([]*Manifest, error) {
	target := heads[r.BranchName()]
	if target == "" || !r.HasSnapshot(target) {
		return nil, nil
	}
	have, err := r.ancestors(r.Latest())
	if err != nil || have[target] && r.seenHeads()[r.BranchName()] == target {
		return nil, err
	}
	team := map[string]bool{} // what the team's branches have
	for _, h := range heads {
		if h == "" || team[h] {
			continue
		}
		if !r.HasSnapshot(h) {
			return nil, nil // can't tell yet
		}
		in, err := r.ancestors(h)
		if err != nil {
			return nil, err
		}
		for id := range in {
			team[id] = true
		}
	}
	// What this copy took in from the team and the team no longer has. A
	// copy from before it kept track: the versions here by others.
	from, others := have, true
	if seen := r.seenHeads()[r.BranchName()]; seen != "" && r.HasSnapshot(seen) {
		if from, err = r.ancestors(seen); err != nil {
			return nil, err
		}
		others = false
	}
	me, _ := r.Identity()
	var out []*Manifest
	for id := range from {
		if team[id] || !have[id] {
			continue
		}
		m, err := r.Load(id)
		if err != nil {
			return nil, err
		}
		if others && (m.AuthorID == "" || m.AuthorID == me) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time > out[j].Time })
	return out, nil
}

// takenBack is TakenBackFrom asking the team: heads are its branches.
func (r *Repo) takenBack(c remote.Backend, heads map[string]string) ([]*Manifest, error) {
	target := heads[r.BranchName()]
	if target == "" {
		return nil, nil
	}
	// Most of the time the team's branch has the version seen last: nothing
	// to fetch.
	if seen := r.seenHeads()[r.BranchName()]; seen == target {
		return nil, nil
	}
	for _, h := range heads {
		if h != "" {
			if err := r.fetchSnapshots(c, h); err != nil {
				return nil, err
			}
		}
	}
	return r.TakenBackFrom(heads)
}

// ErrCantDropTakenBack: the team took back a version this copy can't take
// out by itself.
var ErrCantDropTakenBack = errors.New("a teammate took back a version this copy has, " +
	"and it can't be taken out here (a merge came after it): go to the latest version of the team")

// dropTakenBack takes the versions the team took back out of this copy:
// the project goes to the team's latest version, with the versions
// committed here and not shared put after it, and the uncommitted changes
// kept (merged in, still uncommitted). nil when nothing was taken back.
func (r *Repo) dropTakenBack(c remote.Backend, heads map[string]string, opts MergeOptions) (*SyncResult, error) {
	taken, err := r.takenBack(c, heads)
	if err != nil || len(taken) == 0 {
		return nil, err
	}
	if err := r.guardLatest(); err != nil {
		return nil, err
	}
	target := heads[r.BranchName()]
	drop := map[string]bool{}
	for _, m := range taken {
		drop[m.ID] = true
	}
	team, err := r.ancestors(target)
	if err != nil {
		return nil, err
	}
	// Ours: the versions committed here since, oldest first.
	var ours []*Manifest
	for id := r.Head(); !team[id]; {
		if id == "" {
			return nil, ErrCantDropTakenBack
		}
		m, err := r.Load(id)
		if err != nil {
			return nil, err
		}
		if len(m.Parents) != 1 {
			return nil, ErrCantDropTakenBack
		}
		if !drop[id] {
			ours = append(ours, m)
		}
		id = m.Parents[0]
	}
	slices.Reverse(ours)
	onto, err := r.Load(target)
	if err != nil {
		return nil, err
	}
	head := r.Head()
	res := &SyncResult{Action: "taken-back", From: head, TakenBack: taken}
	onto, res.MergeLog, err = r.replay(c, ours, onto, opts)
	if err != nil {
		return nil, err
	}
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	r.noteTeamHead(c, target)
	if len(changes) > 0 {
		if _, err := r.updateKeepingWork(c, head, onto.ID, opts, res); err != nil {
			return nil, err
		}
	} else {
		r.knowSizes(onto)
		if err := r.fetchObjects(c, onto.Objects()); err != nil {
			return nil, err
		}
		if res.Relinked, err = r.putFiles(onto, onto.ID, onto.ID); err != nil {
			return nil, err
		}
	}
	res.Action, res.To = "taken-back", onto.ID
	return res, nil
}
