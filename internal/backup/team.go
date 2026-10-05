package backup

import (
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// Backing up a team this computer is connected to: the app does it daily
// (desktop/backup.go), `dawgit backup` on demand. Both note the result in
// teams.json (when it went where the app backs the team up) and in the
// team's storage (backups/<member>.json: times only, no paths), so the
// other members know the team is covered.

const (
	// Recent: a member's backup this recent covers the team.
	Recent = 7 * 24 * time.Hour
	// FailingAfter: failed runs only count as failing after this long
	// without a backup (a drive unplugged for a day is normal).
	FailingAfter = 3 * 24 * time.Hour
)

// Storage is team t's storage, if it can be backed up.
func Storage(t teams.Team) (*remote.BucketBackend, error) {
	if !t.Remote.IsStorage() {
		return nil, errors.New("only teams that keep their work in storage (R2, S3) can be backed up")
	}
	b, err := t.Open()
	if err != nil {
		return nil, err
	}
	s3, ok := b.(*remote.BucketBackend)
	if !ok {
		return nil, errors.New("this team's storage can't be backed up")
	}
	return s3, nil
}

// Failing: b's runs have failed for a while.
func Failing(b *teams.Backup) bool {
	return b != nil && !b.Paused && b.LastError != "" &&
		(b.LastSuccess.IsZero() || time.Since(b.LastSuccess) > FailingAfter)
}

// DestOf is where b backs up to.
func DestOf(b *teams.Backup) (Dest, error) {
	if b.Storage != nil {
		return Bucket(*b.Storage)
	}
	if b.Folder == "" {
		return nil, errors.New("no backup folder")
	}
	return Folder(b.Folder), nil
}

// Overlaps: storage cfg is (in) the team's own storage, or holds it: a
// backup there would copy itself, or be lost with the team's storage.
func Overlaps(cfg remote.Config, t teams.Team) bool {
	a, ok1 := remote.StorageOf(cfg)
	b, ok2 := remote.StorageOf(t.Remote)
	if !ok1 || !ok2 || !strings.EqualFold(hostOf(a.Endpoint), hostOf(b.Endpoint)) || a.Bucket != b.Bucket {
		return false
	}
	pa, pb := strings.Trim(a.Folder, "/")+"/", strings.Trim(b.Folder, "/")+"/"
	return strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa)
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil {
		return u.Host
	}
	return endpoint
}

// RunTeam backs up team teamID into d (nil: where this computer backs it
// up). claim: d may be new (empty: claimed now); otherwise it must already
// hold the team's backup, so an unplugged drive fails with ErrMissing
// instead of filling the folder it mounts on.
func RunTeam(teamID string, d Dest, claim bool, progress Progress) (*Report, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	team := *t
	if d == nil {
		if t.Backup == nil {
			return nil, errors.New("this computer doesn't back up this team")
		}
		if d, err = DestOf(t.Backup); err != nil {
			return nil, err
		}
	}
	started := time.Now()
	rep, err := func() (*Report, error) {
		s3, err := Storage(team)
		if err != nil {
			return nil, err
		}
		if claim {
			err = Claim(d, team.ID, team.Name)
		} else {
			err = Claimed(d, team.ID)
		}
		if err != nil {
			return nil, err
		}
		return Run(s3, d, progress)
	}()
	note(teamID, d, started, rep, err)
	return rep, err
}

// isConfigured: d is where b backs up.
func isConfigured(b *teams.Backup, d Dest) bool {
	if b == nil {
		return false
	}
	c, err := DestOf(b)
	return err == nil && c.Kind() == d.Kind() && strings.EqualFold(c.Name(), d.Name())
}

// note records a run: in teams.json, read again (the settings may have
// changed meanwhile), and in the team's storage.
func note(teamID string, d Dest, started time.Time, rep *Report, runErr error) {
	var size int64
	if runErr == nil && rep != nil {
		size = d.Size() // can take a while: not while holding the settings
	}
	record := func(b *teams.Backup) {
		b.LastAttempt = started
		if runErr == nil {
			b.LastSuccess, b.LastError = started, ""
			if rep != nil {
				b.Size = size
			}
		} else {
			b.LastError = runErr.Error()
		}
	}
	var t teams.Team
	b := &teams.Backup{} // a place of its own (dawgit backup run <folder>): only the team hears of it
	if _, err := teams.Update(func(store *teams.Store) error {
		found := store.Find(teamID)
		if found == nil {
			return errors.New("unknown team")
		}
		if isConfigured(found.Backup, d) {
			record(found.Backup)
			b = found.Backup
		} else {
			record(b)
		}
		t = *found
		return nil
	}); err != nil {
		return
	}
	if errors.Is(runErr, ErrOtherTeam) || errors.Is(runErr, ErrNotEmpty) || t.MemberID == "" {
		return // nothing was backed up, or no one to note it for
	}
	if s3, err := Storage(t); err == nil {
		s, _ := s3.BackupStatuses()
		last := s[t.MemberID].LastSuccess
		if b.LastSuccess.After(last) {
			last = b.LastSuccess
		}
		s3.PutBackupStatus(t.MemberID, remote.BackupStatus{Kind: d.Kind(),
			LastSuccess: last, LastAttempt: b.LastAttempt, Failing: Failing(b)})
	}
}

// Announce notes team t's backup on this computer in the team's storage,
// as it stands: for a backup set up before the member had a name (creating
// a team), whose runs couldn't note it then.
func Announce(t teams.Team) error {
	b := t.Backup
	if b == nil || t.MemberID == "" || b.LastAttempt.IsZero() {
		return nil // a run in progress notes it when done
	}
	s3, err := Storage(t)
	if err != nil {
		return err
	}
	kind := "folder"
	if b.Storage != nil {
		kind = "s3"
	}
	return s3.PutBackupStatus(t.MemberID, remote.BackupStatus{Kind: kind,
		LastSuccess: b.LastSuccess, LastAttempt: b.LastAttempt, Failing: Failing(b)})
}

// Member is a member's backups of a team, as noted in its storage.
type Member struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	LastSuccess time.Time `json:"lastSuccess"`
	LastAttempt time.Time `json:"lastAttempt"`
	Failing     bool      `json:"failing"`
}

// Members lists who backs up team t, by name.
func Members(t teams.Team) ([]Member, error) {
	s3, err := Storage(t)
	if err != nil {
		return nil, err
	}
	statuses, err := s3.BackupStatuses()
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if ms, err := s3.Members(); err == nil {
		for _, m := range ms {
			names[m.ID] = m.Name
		}
	}
	out := []Member{}
	for id, s := range statuses {
		name := names[id]
		if name == "" {
			name = "?"
		}
		out = append(out, Member{ID: id, Name: name, LastSuccess: s.LastSuccess, LastAttempt: s.LastAttempt,
			Failing: s.Failing && time.Since(s.LastSuccess) > FailingAfter})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Covered: some member backed up team t lately.
func Covered(members []Member) bool {
	for _, m := range members {
		if time.Since(m.LastSuccess) < Recent {
			return true
		}
	}
	return false
}
