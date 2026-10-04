package desktop

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"sort"
	"sync"
	"time"

	"dawgit/internal/backup"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// A member can back up the whole team's storage to a folder (an external
// drive, a NAS) once a day while DAWGit runs (internal/backup). Each member's
// last backup is noted in the team's storage (no paths), so the others know
// the team is covered and aren't reminded to set one up.

const (
	backupEvery = 24 * time.Hour
	// backupRetry: after a failed run (drive unplugged…), try again after.
	backupRetry = time.Hour
	// backupCheck: how often the schedule is looked at.
	backupCheck = 15 * time.Minute
	// backupRecent: a member's backup this recent covers the team.
	backupRecent = 7 * 24 * time.Hour
	// backupRemind: the reminder to set one up comes back after.
	backupRemind = 7 * 24 * time.Hour
	// backupFailingAfter: failed runs are only shown after this long
	// without a backup (a drive unplugged for a day is normal).
	backupFailingAfter = 3 * 24 * time.Hour
)

// backupRun is a run in progress.
type backupRun struct {
	Done, Total int64
}

var (
	backupMu      sync.Mutex
	backupRunning = map[string]*backupRun{} // team id
)

// BackupInfo is a team's backup, for Team Settings.
type BackupInfo struct {
	// Supported: the team keeps its work in storage (R2/S3).
	Supported bool   `json:"supported"`
	Folder    string `json:"folder"` // "" when this computer doesn't back up
	Paused    bool   `json:"paused"`
	Running   bool   `json:"running"`
	Done      int64  `json:"done"`
	Total     int64  `json:"total"`
	// LastSuccess and LastAttempt: RFC 3339, "" for never.
	LastSuccess string `json:"lastSuccess"`
	LastAttempt string `json:"lastAttempt"`
	// Problem: why the last run failed: "missing" (the folder isn't there:
	// a drive unplugged?), "other" (Error says), "" (it didn't).
	Problem string `json:"problem"`
	Error   string `json:"error"`
	Failing bool   `json:"failing"` // failing for a while: worth a warning
	Size    int64  `json:"size"`
	// Others: the team's other members who back it up.
	Others []MemberBackup `json:"others"`
}

type MemberBackup struct {
	Name        string `json:"name"`
	LastSuccess string `json:"lastSuccess"`
	Failing     bool   `json:"failing"`
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func storageOf(t teams.Team) (*remote.S3Backend, error) {
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

func backupFailing(b *teams.Backup) bool {
	return b != nil && !b.Paused && b.LastError != "" &&
		(b.LastSuccess.IsZero() || time.Since(b.LastSuccess) > backupFailingAfter)
}

// BackupInfo says how team teamID is backed up.
func (a *App) BackupInfo(teamID string) (*BackupInfo, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	info := &BackupInfo{Supported: t.Remote.IsStorage(), Others: []MemberBackup{}}
	if b := t.Backup; b != nil {
		info.Folder, info.Paused, info.Size = b.Folder, b.Paused, b.Size
		info.LastSuccess, info.LastAttempt = stamp(b.LastSuccess), stamp(b.LastAttempt)
		info.Error, info.Failing = b.LastError, backupFailing(b)
		if b.LastError == errMissing.Error() {
			info.Problem = "missing"
		} else if b.LastError != "" {
			info.Problem = "other"
		}
	}
	backupMu.Lock()
	if r := backupRunning[teamID]; r != nil {
		info.Running, info.Done, info.Total = true, r.Done, r.Total
	}
	backupMu.Unlock()
	if !info.Supported {
		return info, nil
	}
	if s3, err := storageOf(*t); err == nil {
		statuses, _ := s3.BackupStatuses()
		names := map[string]string{}
		if ms, err := s3.Members(); err == nil {
			for _, m := range ms {
				names[m.ID] = m.Name
			}
		}
		for id, s := range statuses {
			if id == t.MemberID {
				continue
			}
			name := names[id]
			if name == "" {
				name = "?"
			}
			info.Others = append(info.Others, MemberBackup{Name: name, LastSuccess: stamp(s.LastSuccess),
				Failing: s.Failing && time.Since(s.LastSuccess) > backupFailingAfter})
		}
		sort.Slice(info.Others, func(i, j int) bool { return info.Others[i].Name < info.Others[j].Name })
	}
	return info, nil
}

// SetBackupFolder makes folder where this computer backs up team teamID,
// and starts a backup. The problem ("" when none) is why the folder can't
// be used: "other-team" (it holds another team's backup), "not-empty".
func (a *App) SetBackupFolder(teamID, folder string) (string, error) {
	store, err := teams.Load()
	if err != nil {
		return "", err
	}
	t := store.Find(teamID)
	if t == nil {
		return "", errors.New("unknown team")
	}
	if _, err := storageOf(*t); err != nil {
		return "", err
	}
	switch err := backup.Claim(folder, t.ID, t.Name); {
	case errors.Is(err, backup.ErrOtherTeam):
		return "other-team", nil
	case errors.Is(err, backup.ErrNotEmpty):
		return "not-empty", nil
	case err != nil:
		return "", err
	}
	if t.Backup == nil || t.Backup.Folder != folder {
		t.Backup = &teams.Backup{Folder: folder}
	}
	t.Backup.Paused = false
	if err := store.Save(); err != nil {
		return "", err
	}
	go a.backUp(teamID)
	return "", nil
}

// BackUpNow starts a backup of team teamID now.
func (a *App) BackUpNow(teamID string) {
	go a.backUp(teamID)
}

// PauseBackup stops (or restarts) the daily backups of team teamID.
func (a *App) PauseBackup(teamID string, paused bool) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(teamID)
	if t == nil || t.Backup == nil {
		return errors.New("this team isn't backed up on this computer")
	}
	t.Backup.Paused = paused
	return store.Save()
}

// StopBackup: this computer no longer backs up team teamID. The backup
// folder is left as it is.
func (a *App) StopBackup(teamID string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(teamID)
	if t == nil {
		return errors.New("unknown team")
	}
	t.Backup = nil
	if err := store.Save(); err != nil {
		return err
	}
	if s3, err := storageOf(*t); err == nil && t.MemberID != "" {
		s3.DeleteBackupStatus(t.MemberID)
	}
	return nil
}

// BackupReminder: whether to suggest setting up a backup of team teamID:
// a storage team nobody has backed up lately, not put off this week.
func (a *App) BackupReminder(teamID string) bool {
	store, err := teams.Load()
	if err != nil {
		return false
	}
	t := store.Find(teamID)
	if t == nil || t.Backup != nil || t.MemberID == "" || time.Since(t.BackupHushed) < backupRemind {
		return false
	}
	s3, err := storageOf(*t)
	if err != nil {
		return false
	}
	statuses, err := s3.BackupStatuses()
	if err != nil {
		return false // can't tell: don't nag
	}
	for _, s := range statuses {
		if time.Since(s.LastSuccess) < backupRecent {
			return false
		}
	}
	return true
}

// HushBackupReminder puts the reminder off for a week.
func (a *App) HushBackupReminder(teamID string) error {
	store, err := teams.Load()
	if err != nil {
		return err
	}
	t := store.Find(teamID)
	if t == nil {
		return errors.New("unknown team")
	}
	t.BackupHushed = time.Now()
	return store.Save()
}

// backUpOnSchedule backs up every team due, while the app runs.
func (a *App) backUpOnSchedule(ctx context.Context) {
	wait := 2 * time.Minute // let the app start first
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = backupCheck
		store, err := teams.Load()
		if err != nil {
			continue
		}
		for _, t := range store.Teams {
			b := t.Backup
			if b == nil || b.Paused || time.Since(b.LastSuccess) < backupEvery ||
				(b.LastError != "" && time.Since(b.LastAttempt) < backupRetry) {
				continue
			}
			a.backUp(t.ID)
		}
	}
}

var errMissing = errors.New("the backup folder isn't there")

// backUp backs up team teamID now (unless it's being backed up).
func (a *App) backUp(teamID string) {
	backupMu.Lock()
	if backupRunning[teamID] != nil {
		backupMu.Unlock()
		return
	}
	run := &backupRun{}
	backupRunning[teamID] = run
	backupMu.Unlock()
	defer func() {
		backupMu.Lock()
		delete(backupRunning, teamID)
		backupMu.Unlock()
		a.emitBackup(teamID)
	}()

	store, err := teams.Load()
	if err != nil {
		return
	}
	t := store.Find(teamID)
	if t == nil || t.Backup == nil {
		return
	}
	team, folder := *t, t.Backup.Folder
	a.emitBackup(teamID)

	started := time.Now()
	var size int64
	err = func() error {
		if err := backup.Claimed(folder, teamID); errors.Is(err, fs.ErrNotExist) {
			return errMissing
		} else if err != nil {
			return err
		}
		s3, err := storageOf(team)
		if err != nil {
			return err
		}
		var last time.Time
		_, err = backup.Run(s3, folder, func(done, total int64) {
			backupMu.Lock()
			run.Done, run.Total = done, total
			backupMu.Unlock()
			if time.Since(last) > 500*time.Millisecond {
				last = time.Now()
				a.emitBackup(teamID)
			}
		})
		if err == nil {
			size = backup.Size(folder)
		}
		return err
	}()
	if err != nil {
		log.Printf("backup %s: %v", teamID, err)
	}

	// Noted again on a fresh copy of the settings (they may have changed).
	store, lerr := teams.Load()
	if lerr != nil {
		return
	}
	t = store.Find(teamID)
	if t == nil || t.Backup == nil || t.Backup.Folder != folder {
		return
	}
	b := t.Backup
	b.LastAttempt = started
	if err == nil {
		b.LastSuccess, b.LastError, b.Size = started, "", size
	} else {
		b.LastError = err.Error()
	}
	store.Save()
	if s3, serr := storageOf(*t); serr == nil && t.MemberID != "" {
		s3.PutBackupStatus(t.MemberID, remote.BackupStatus{Kind: "folder",
			LastSuccess: b.LastSuccess, LastAttempt: b.LastAttempt, Failing: backupFailing(b)})
	}
}

func (a *App) emitBackup(teamID string) {
	if a.emit != nil {
		a.emit("backup", teamID)
	}
}
