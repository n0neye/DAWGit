package desktop

import (
	"context"
	"errors"
	"log"
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
	// backupRemind: the reminder to set one up comes back after.
	backupRemind = 7 * 24 * time.Hour
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
	Supported bool `json:"supported"`
	// Kind: "folder" or "s3"; Folder: where (the folder, or the storage's
	// address, bucket and folder; no keys); "" when this computer doesn't
	// back up.
	Kind    string `json:"kind"`
	Folder  string `json:"folder"`
	Paused  bool   `json:"paused"`
	Running bool   `json:"running"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	// LastSuccess and LastAttempt: RFC 3339, "" for never.
	LastSuccess string `json:"lastSuccess"`
	LastAttempt string `json:"lastAttempt"`
	// Problem: why the last run failed: "missing" (the folder isn't there:
	// a drive unplugged?), "other" (Error says), "" (it didn't).
	Problem string `json:"problem"`
	Error   string `json:"error"`
	Failing bool   `json:"failing"` // failing for a while: worth a warning
	Size    int64  `json:"size"`
	// Others: the team's other members who back it up; Covered: one of
	// them did lately.
	Others  []MemberBackup `json:"others"`
	Covered bool           `json:"covered"`
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
		if d, err := backup.DestOf(b); err == nil {
			info.Kind, info.Folder = d.Kind(), d.Name()
		}
		info.Paused, info.Size = b.Paused, b.Size
		info.LastSuccess, info.LastAttempt = stamp(b.LastSuccess), stamp(b.LastAttempt)
		info.Error, info.Failing = b.LastError, backup.Failing(b)
		if b.LastError == backup.ErrMissing.Error() {
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
	if members, err := backup.Members(*t); err == nil {
		var others []backup.Member
		for _, m := range members {
			if m.ID != t.MemberID {
				others = append(others, m)
				info.Others = append(info.Others, MemberBackup{Name: m.Name, LastSuccess: stamp(m.LastSuccess), Failing: m.Failing})
			}
		}
		info.Covered = backup.Covered(others)
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
	if _, err := backup.Storage(*t); err != nil {
		return "", err
	}
	return a.useBackup(store, t, backup.Folder(folder), teams.Backup{Folder: folder})
}

// SetBackupStorage makes S3-compatible storage (another bucket, or a folder
// of one) where this computer backs up team teamID, and starts a backup.
// Problems as SetBackupFolder's, and "same-storage": it is (in) the team's
// own storage.
func (a *App) SetBackupStorage(teamID string, s remote.Storage) (string, error) {
	store, err := teams.Load()
	if err != nil {
		return "", err
	}
	t := store.Find(teamID)
	if t == nil {
		return "", errors.New("unknown team")
	}
	if _, err := backup.Storage(*t); err != nil {
		return "", err
	}
	cfg, err := s.Config()
	if err != nil {
		return "", err
	}
	if backup.Overlaps(cfg, *t) {
		return "same-storage", nil
	}
	if err := remote.CheckBackup(cfg); err != nil {
		return "", err
	}
	d, err := backup.Bucket(cfg)
	if err != nil {
		return "", err
	}
	return a.useBackup(store, t, d, teams.Backup{Storage: &cfg})
}

// BackupStorage is this computer's backup storage for team teamID, to edit
// (nil when it backs up to a folder, or not at all).
func (a *App) BackupStorage(teamID string) (*remote.Storage, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil || t.Backup == nil || t.Backup.Storage == nil {
		return nil, nil
	}
	s, ok := remote.StorageOf(*t.Backup.Storage)
	if !ok {
		return nil, nil
	}
	return &s, nil
}

// useBackup claims d for team t and backs up there from now on.
func (a *App) useBackup(store *teams.Store, t *teams.Team, d backup.Dest, b teams.Backup) (string, error) {
	switch err := backup.Claim(d, t.ID, t.Name); {
	case errors.Is(err, backup.ErrOtherTeam):
		return "other-team", nil
	case errors.Is(err, backup.ErrNotEmpty):
		return "not-empty", nil
	case err != nil:
		return "", err
	}
	if old := t.Backup; old != nil {
		if c, err := backup.DestOf(old); err == nil && c.Kind() == d.Kind() && c.Name() == d.Name() {
			b.LastSuccess, b.LastAttempt, b.Size = old.LastSuccess, old.LastAttempt, old.Size
		}
	}
	t.Backup = &b
	if err := store.Save(); err != nil {
		return "", err
	}
	go a.backUp(t.ID)
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
	if s3, err := backup.Storage(*t); err == nil && t.MemberID != "" {
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
	members, err := backup.Members(*t)
	return err == nil && !backup.Covered(members) // can't tell: don't nag
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

	a.emitBackup(teamID)
	var last time.Time
	_, err := backup.RunTeam(teamID, nil, false, func(done, total int64) {
		backupMu.Lock()
		run.Done, run.Total = done, total
		backupMu.Unlock()
		if time.Since(last) > 500*time.Millisecond {
			last = time.Now()
			a.emitBackup(teamID)
		}
	})
	if err != nil {
		log.Printf("backup %s: %v", teamID, err)
	}
}

func (a *App) emitBackup(teamID string) {
	if a.emit != nil {
		a.emit("backup", teamID)
	}
}

// RestorePlan is what restoring a backup into the team's storage would
// bring back (see backup.MakePlan).
type RestorePlan struct {
	Where    string               `json:"where"` // the backup: a folder, or a bucket's address
	Team     string               `json:"team"`  // whose backup it is (the team's name then)
	Run      string               `json:"run"`   // "" for the latest
	Runs     []string             `json:"runs"`  // newest first, as 20261004-153000 (UTC)
	Projects []backup.PlanProject `json:"projects"`
	Branches int                  `json:"branches"`
	Files    int                  `json:"files"`
	Bytes    int64                `json:"bytes"`
}

// restoreSource: the folder given, or where this computer backs team t up.
func restoreSource(t teams.Team, folder string) (backup.Dest, error) {
	if folder != "" {
		return backup.Folder(folder), nil
	}
	if t.Backup == nil {
		return nil, errors.New("this computer doesn't back up this team: choose the backup's folder")
	}
	return backup.DestOf(t.Backup)
}

func (a *App) restoreParts(teamID, folder, run string) (backup.Dest, *backup.Plan, *teams.Team, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, nil, nil, errors.New("unknown team")
	}
	src, err := restoreSource(*t, folder)
	if err != nil {
		return nil, nil, nil, err
	}
	s3, err := backup.Storage(*t)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := backup.MakePlan(src, s3, run)
	if errors.Is(err, backup.ErrNotBackup) && folder == "" {
		err = backup.ErrMissing // its drive is unplugged
	}
	return src, p, t, err
}

// RestorePlan says what restoring a backup (folder "": this computer's) as
// of run ("": the latest) would bring back into team teamID's storage.
func (a *App) RestorePlan(teamID, folder, run string) (*RestorePlan, error) {
	src, p, _, err := a.restoreParts(teamID, folder, run)
	if err != nil {
		return nil, err
	}
	return &RestorePlan{Where: src.Name(), Team: p.Team, Run: p.Run, Runs: p.Runs, Projects: p.Projects,
		Branches: p.Branches, Files: p.Files, Bytes: p.Bytes}, nil
}

// RestoreProgress is sent while a restore runs (event "restore").
type RestoreProgress struct {
	TeamID string `json:"teamId"`
	Done   int64  `json:"done"`
	Total  int64  `json:"total"`
}

// Restore brings back from a backup what team teamID's storage lacks (see
// RestorePlan), never overwriting anything; it returns how many files it
// copied.
func (a *App) Restore(teamID, folder, run string) (int, error) {
	src, p, t, err := a.restoreParts(teamID, folder, run)
	if err != nil {
		return 0, err
	}
	s3, err := backup.Storage(*t)
	if err != nil {
		return 0, err
	}
	var last time.Time
	rep, err := backup.Restore(src, s3, p, func(done, total int64) {
		if a.emit != nil && (time.Since(last) > 300*time.Millisecond || done == total) {
			last = time.Now()
			a.emit("restore", RestoreProgress{TeamID: teamID, Done: done, Total: total})
		}
	})
	forgetNames(t.Remote.URL)
	if rep == nil {
		return 0, err
	}
	return rep.Copied, err
}
