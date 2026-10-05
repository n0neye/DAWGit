package remote

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"
)

// Team features: what a team turned on that changes what it stores (a new
// kind of record, say). They are listed in its info (team.json); a DAWGit
// that doesn't know one of them stops before working with the team instead
// of misreading or overwriting what it doesn't understand. This is how
// Nightly features that change storage stay off for teams with Stable
// members until the team turns them on.

var (
	featuresMu sync.Mutex
	known      = map[string]bool{}
)

// RegisterFeature says this build understands feature name (see
// dawgit/ext).
func RegisterFeature(name string) {
	featuresMu.Lock()
	known[name] = true
	featuresMu.Unlock()
}

// Missing lists the team's features this build doesn't know.
func Missing(info TeamInfo) []string {
	featuresMu.Lock()
	defer featuresMu.Unlock()
	var out []string
	for _, f := range info.Features {
		if !known[f] {
			out = append(out, f)
		}
	}
	return out
}

// ErrTeamFeatures: the team uses features this DAWGit doesn't have.
type ErrTeamFeatures struct{ Missing []string }

func (e *ErrTeamFeatures) Error() string {
	return fmt.Sprintf("this team uses features this DAWGit doesn't have (%s): update DAWGit, or switch to "+
		"the Nightly channel (Settings → Updates)", strings.Join(e.Missing, ", "))
}

// featureChecks remembers recent checks (per team address): a team's
// features rarely change, and every operation opens the team.
var featureChecks sync.Map // url -> time.Time of a check that passed

// CheckFeatures fails with *ErrTeamFeatures when the team behind b (at url)
// uses features this build doesn't know. A team that can't be asked right
// now passes: the operation itself will say it can't reach the team.
func CheckFeatures(b Backend, url string) error {
	if t, ok := featureChecks.Load(url); ok && time.Since(t.(time.Time)) < 10*time.Minute {
		return nil
	}
	info, err := b.Info()
	if err != nil {
		return nil
	}
	if m := Missing(info); len(m) > 0 {
		return &ErrTeamFeatures{Missing: m}
	}
	featureChecks.Store(url, time.Now())
	return nil
}

// Rename sets the team's name, keeping the rest of its info.
func Rename(b Backend, name string) error {
	info, err := b.Info()
	if err != nil {
		return err // never write the info back without what it holds
	}
	info.Name = name
	return b.SetInfo(info)
}

// EnableFeature turns feature name on for the team: from then on, DAWGits
// that don't know it stop before working with it.
func EnableFeature(b Backend, name string) error {
	info, err := b.Info()
	if err != nil {
		return err
	}
	if slices.Contains(info.Features, name) {
		return nil
	}
	info.Features = append(info.Features, name)
	return b.SetInfo(info)
}

// RenameProject renames a project for the team, keeping the rest of its
// record (fields a newer DAWGit may have added).
func RenameProject(b Backend, pid, name string) error {
	ps, err := b.Projects()
	if err != nil {
		return err
	}
	p := Project{ID: pid}
	for _, q := range ps {
		if q.ID == pid {
			p = q
		}
	}
	p.Name = name
	return b.PutProject(p)
}

// RenameMember adds a member or renames one, keeping the rest of their
// record.
func RenameMember(b Backend, id, name string) error {
	ms, err := b.Members()
	if err != nil && !errors.Is(err, ErrOldServer) {
		return err
	}
	m := Member{ID: id}
	for _, q := range ms {
		if q.ID == id {
			m = q
		}
	}
	m.Name = name
	return b.PutMember(m)
}
