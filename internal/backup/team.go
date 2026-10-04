package backup

import (
	"errors"
	"io/fs"
	"sort"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// Backing up a team this computer is connected to: the app does it daily
// (desktop/backup.go), `dawgit backup` on demand. Both note the result in
// teams.json (when the folder is the team's backup folder there) and in the
// team's storage (backups/<member>.json: times only, no paths), so the
// other members know the team is covered.

const (
	// Recent: a member's backup this recent covers the team.
	Recent = 7 * 24 * time.Hour
	// FailingAfter: failed runs only count as failing after this long
	// without a backup (a drive unplugged for a day is normal).
	FailingAfter = 3 * 24 * time.Hour
)

// ErrMissing: the backup folder isn't there (a drive unplugged?).
var ErrMissing = errors.New("the backup folder isn't there")

// Storage is team t's storage, if it can be backed up.
func Storage(t teams.Team) (*remote.S3Backend, error) {
	if !t.Remote.IsStorage() {
		return nil, errors.New("only teams that keep their work in storage (R2, S3) can be backed up")
	}
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	s3, ok := b.(*remote.S3Backend)
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

// RunTeam backs up team teamID into folder. claim: the folder may be new
// (empty, or made); otherwise it must already hold the team's backup, so an
// unplugged drive fails with ErrMissing instead of filling the folder it
// mounts on.
func RunTeam(teamID, folder string, claim bool, progress Progress) (*Report, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	team := *t
	started := time.Now()
	rep, err := func() (*Report, error) {
		s3, err := Storage(team)
		if err != nil {
			return nil, err
		}
		if claim {
			err = Claim(folder, team.ID, team.Name)
		} else if err = Claimed(folder, team.ID); errors.Is(err, fs.ErrNotExist) {
			err = ErrMissing
		}
		if err != nil {
			return nil, err
		}
		return Run(s3, folder, progress)
	}()
	note(teamID, folder, started, rep, err)
	return rep, err
}

// note records a run: in teams.json, read again (the settings may have
// changed meanwhile), and in the team's storage.
func note(teamID, folder string, started time.Time, rep *Report, runErr error) {
	store, err := teams.Load()
	if err != nil {
		return
	}
	t := store.Find(teamID)
	if t == nil {
		return
	}
	b := t.Backup
	if b == nil || b.Folder != folder {
		// A folder of its own (dawgit backup): only the team hears of it.
		b = &teams.Backup{Folder: folder}
	} else {
		defer store.Save()
	}
	b.LastAttempt = started
	if runErr == nil {
		b.LastSuccess, b.LastError = started, ""
		if rep != nil {
			b.Size = Size(folder)
		}
	} else {
		b.LastError = runErr.Error()
	}
	if errors.Is(runErr, ErrOtherTeam) || errors.Is(runErr, ErrNotEmpty) || t.MemberID == "" {
		return // nothing was backed up, or no one to note it for
	}
	if s3, err := Storage(*t); err == nil {
		s, _ := s3.BackupStatuses()
		last := s[t.MemberID].LastSuccess
		if b.LastSuccess.After(last) {
			last = b.LastSuccess
		}
		s3.PutBackupStatus(t.MemberID, remote.BackupStatus{Kind: "folder",
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
	return s3.PutBackupStatus(t.MemberID, remote.BackupStatus{Kind: "folder",
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
