package desktop

import (
	"context"
	"errors"
	"log"
	"strings"
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
	info := &BackupInfo{Supported: t.Remote.IsStorage()}
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
	return info, nil
}

// BackupOthers is how the team's other members back it up (asks the team:
// BackupInfo shows this computer's at once, then this).
type BackupOthers struct {
	Others []MemberBackup `json:"others"`
	// Covered: one of them backed it up lately.
	Covered bool `json:"covered"`
}

// BackupOthers says how team teamID's other members back it up.
func (a *App) BackupOthers(teamID string) (*BackupOthers, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, errors.New("unknown team")
	}
	out := &BackupOthers{Others: []MemberBackup{}}
	members, err := backup.Members(*t)
	if err != nil {
		return nil, err
	}
	var others []backup.Member
	for _, m := range members {
		if m.ID != t.MemberID {
			others = append(others, m)
			out.Others = append(out.Others, MemberBackup{Name: m.Name, LastSuccess: stamp(m.LastSuccess), Failing: m.Failing})
		}
	}
	out.Covered = backup.Covered(others)
	return out, nil
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
	return a.useBackup(t, backup.Folder(folder), teams.Backup{Folder: folder})
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
	return a.useBackup(t, d, teams.Backup{Storage: &cfg})
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
func (a *App) useBackup(t *teams.Team, d backup.Dest, b teams.Backup) (string, error) {
	switch err := backup.Claim(d, t.ID, t.Name); {
	case errors.Is(err, backup.ErrOtherTeam):
		return "other-team", nil
	case errors.Is(err, backup.ErrNotEmpty):
		return "not-empty", nil
	case err != nil:
		return "", err
	}
	if _, err := updateTeam(t.ID, func(_ *teams.Store, t *teams.Team) error {
		if old := t.Backup; old != nil {
			if c, err := backup.DestOf(old); err == nil && c.Kind() == d.Kind() && c.Name() == d.Name() {
				b.LastSuccess, b.LastAttempt, b.Size = old.LastSuccess, old.LastAttempt, old.Size
			}
		}
		t.Backup = &b
		return nil
	}); err != nil {
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
	_, err := updateTeam(teamID, func(_ *teams.Store, t *teams.Team) error {
		if t.Backup == nil {
			return errors.New("this team isn't backed up on this computer")
		}
		t.Backup.Paused = paused
		return nil
	})
	return err
}

// StopBackup: this computer no longer backs up team teamID. The backup
// folder is left as it is.
func (a *App) StopBackup(teamID string) error {
	t, err := updateTeam(teamID, func(_ *teams.Store, t *teams.Team) error {
		t.Backup = nil
		return nil
	})
	if err != nil {
		return err
	}
	if s3, err := backup.Storage(t); err == nil && t.MemberID != "" {
		s3.DeleteBackupStatus(t.MemberID)
	}
	return nil
}

// BackupReminder: whether to suggest setting up a backup of team teamID:
// a storage team in use for a while (settled), nobody has backed up lately,
// not put off this week.
func (a *App) BackupReminder(teamID string) bool {
	store, err := teams.Load()
	if err != nil {
		return false
	}
	t := store.Find(teamID)
	if t == nil || t.Backup != nil || t.MemberID == "" || time.Since(t.BackupHushed) < backupRemind ||
		!settled(store, *t, time.Now()) {
		return false
	}
	members, err := backup.Members(*t)
	return err == nil && !backup.Covered(members) // can't tell: don't nag
}

// HushBackupReminder puts the reminder off for a week.
func (a *App) HushBackupReminder(teamID string) error {
	_, err := updateTeam(teamID, func(_ *teams.Store, t *teams.Team) error {
		t.BackupHushed = time.Now()
		return nil
	})
	return err
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

// RestoreSource is the backup to restore from: a folder, S3-compatible
// storage (any bucket a backup went to, e.g. from another computer; its
// keys are used for this restore only, and reading is enough), or, with
// neither, where this computer backs the team up.
type RestoreSource struct {
	Folder  string          `json:"folder"`
	Storage *remote.Storage `json:"storage"`
}

// ErrOwnStorage: the backup to restore from is the team's own storage.
var ErrOwnStorage = errors.New("that is where the team keeps its work: choose where its backup is")

func restoreSource(t teams.Team, from RestoreSource) (backup.Dest, error) {
	switch {
	case from.Folder != "":
		return backup.Folder(from.Folder), nil
	case from.Storage != nil:
		cfg, err := from.Storage.Config()
		if err != nil {
			return nil, err
		}
		if backup.Overlaps(cfg, t) {
			return nil, ErrOwnStorage
		}
		return backup.Bucket(cfg)
	case t.Backup == nil:
		return nil, errors.New("this computer doesn't back up this team: choose the backup's folder or bucket")
	}
	return backup.DestOf(t.Backup)
}

func (a *App) restoreParts(teamID string, from RestoreSource, run string) (backup.Dest, *backup.Plan, *teams.Team, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	t := store.Find(teamID)
	if t == nil {
		return nil, nil, nil, errors.New("unknown team")
	}
	src, err := restoreSource(*t, from)
	if err != nil {
		return nil, nil, nil, err
	}
	s3, err := backup.Storage(*t)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := backup.MakePlan(src, s3, run)
	if errors.Is(err, backup.ErrNotBackup) && from.Folder == "" && from.Storage == nil {
		err = backup.ErrMissing // its drive is unplugged
	}
	return src, p, t, err
}

// RestorePlan says what restoring a backup (see RestoreSource) as of run
// ("": the latest) would bring back into team teamID's storage.
func (a *App) RestorePlan(teamID string, from RestoreSource, run string) (*RestorePlan, error) {
	src, p, _, err := a.restoreParts(teamID, from, run)
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
func (a *App) Restore(teamID string, from RestoreSource, run string) (int, error) {
	src, p, t, err := a.restoreParts(teamID, from, run)
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

// A team is settled on this computer once it has been here settleAfter or
// has settleProjects projects: someone trying DAWGit out isn't reminded of
// what could go wrong (see docs/ux-principles.md). Teams from before
// their joining was noted are settled.
const (
	settleAfter    = 3 * 24 * time.Hour
	settleProjects = 3
)

func settled(store *teams.Store, t teams.Team, now time.Time) bool {
	if t.Added.IsZero() || now.Sub(t.Added) >= settleAfter {
		return true
	}
	ids := map[string]bool{}
	for key := range store.Projects {
		if id, ok := strings.CutPrefix(key, t.ID+"/"); ok {
			ids[id] = true
		}
	}
	for _, p := range lastTeamProjects(t.ID) {
		ids[p.ID] = true
	}
	return len(ids) >= settleProjects
}
