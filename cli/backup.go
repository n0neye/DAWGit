package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dawgit/internal/backup"
	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/teams"
)

// dawgit backup: back up a team's storage into a folder (add-only, see
// internal/backup), the way the app does daily. For a scheduled task on a
// NAS or an agent: `dawgit backup run` backs up to the folder chosen in the
// app; `dawgit backup run <folder>` to any folder.

const backupUsage = "usage: dawgit backup run [folder] | status | restore [folder | connection code] [--run TIME] [--preview]  [--team NAME]"

type backupRunJSON struct {
	Team string `json:"team"`
	// Kind: "folder" or "s3"; Folder: where (for "s3": address/bucket/folder).
	Kind        string `json:"kind"`
	Folder      string `json:"folder"`
	Run         string `json:"run"` // the run's record: <folder>/runs/<run>.json
	Keys        int    `json:"keys"`
	Copied      int    `json:"copied"`
	CopiedBytes int64  `json:"copied_bytes"`
	TotalBytes  int64  `json:"total_bytes"`
}

type backupStatusJSON struct {
	Team      string `json:"team"`
	Supported bool   `json:"supported"`
	// Where this computer backs the team up (the app's choice): Kind
	// "folder" or "s3", and the folder or address/bucket/folder; "" if nowhere.
	Kind        string             `json:"kind,omitempty"`
	Folder      string             `json:"folder"`
	Paused      bool               `json:"paused,omitempty"`
	LastSuccess string             `json:"last_success,omitempty"`
	LastAttempt string             `json:"last_attempt,omitempty"`
	Error       string             `json:"error,omitempty"`
	Failing     bool               `json:"failing"`
	Members     []backupMemberJSON `json:"members"` // everyone who backs the team up
	Covered     bool               `json:"covered"` // one of them did in the last 7 days
}

type backupMemberJSON struct {
	Name        string `json:"name"`
	LastSuccess string `json:"last_success,omitempty"`
	Failing     bool   `json:"failing"`
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func cmdBackup(args []string) error {
	if len(args) == 0 {
		return usageError(backupUsage)
	}
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	teamName := fs.String("team", "", "the team's name or id (default: this project's team, or the current one)")
	run := fs.String("run", "", "restore as of this backup run (dawgit backup restore --preview lists them)")
	preview := fs.Bool("preview", false, "restore: only say what would come back")
	pos, err := parseArgs(fs, args[1:])
	if err != nil {
		return err
	}
	t, err := backupTeam(*teamName)
	if err != nil {
		return err
	}
	switch {
	case args[0] == "run" && len(pos) <= 1:
		return backupRun(t, pos)
	case args[0] == "status" && len(pos) == 0:
		return backupStatus(t)
	case args[0] == "restore" && len(pos) <= 1:
		return backupRestore(t, pos, *run, *preview)
	}
	return usageError(backupUsage)
}

// backupTeam: the team named, else the team of the project here, else the
// current one.
func backupTeam(name string) (*teams.Team, error) {
	store, err := teams.Load()
	if err != nil {
		return nil, err
	}
	if name != "" {
		for i, t := range store.Teams {
			if t.ID == name || strings.EqualFold(t.Name, name) {
				return &store.Teams[i], nil
			}
		}
		return nil, fmt.Errorf("not connected to a team %q (dawgit teams lists them)", name)
	}
	if r, err := project.Open("."); err == nil {
		if t, err := r.Team(); err == nil && t != nil {
			return t, nil
		}
	}
	if t := store.Find(store.Current); t != nil {
		return t, nil
	}
	if len(store.Teams) == 1 {
		return &store.Teams[0], nil
	}
	return nil, usageError("which team? add --team NAME (dawgit teams lists them)")
}

func backupRun(t *teams.Team, pos []string) error {
	var d backup.Dest // nil: where the app backs the team up
	claim := false
	if len(pos) == 1 {
		abs, err := filepath.Abs(pos[0])
		if err != nil {
			return err
		}
		// The app's own folder must be there already (an unplugged drive).
		if t.Backup == nil || t.Backup.Storage != nil || !sameFolder(abs, t.Backup.Folder) {
			d, claim = backup.Folder(abs), true
		}
	} else if t.Backup == nil {
		return usageError("this computer doesn't back up %q: give a folder (dawgit backup run <folder>)", t.Name)
	}
	if d == nil {
		var err error
		if d, err = backup.DestOf(t.Backup); err != nil {
			return err
		}
	}
	var last time.Time
	progress := func(done, total int64) {
		if !jsonMode && total > 64<<20 && time.Since(last) > 2*time.Second {
			last = time.Now()
			fmt.Fprintf(os.Stderr, "  %d of %d MB\n", done>>20, total>>20)
		}
	}
	rep, err := backup.RunTeam(t.ID, d, claim, progress)
	if err != nil {
		return backupError(err)
	}
	out := backupRunJSON{Team: t.Name, Kind: d.Kind(), Folder: d.Name(), Run: rep.Run, Keys: rep.Keys, Copied: rep.Copied,
		CopiedBytes: rep.CopiedBytes, TotalBytes: rep.TotalBytes}
	result("backup", out, func() {
		fmt.Printf("backed up %q to %s: %d new files (%.1f MB); %d files (%.1f MB) in all\n", t.Name, d.Name(),
			rep.Copied, float64(rep.CopiedBytes)/(1<<20), rep.Keys, float64(rep.TotalBytes)/(1<<20))
	})
	return nil
}

func sameFolder(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func backupError(err error) error {
	for _, c := range []struct {
		err        error
		code, hint string
	}{
		{backup.ErrMissing, "backup_folder_missing", "connect the drive (or NAS) the backup folder is on; for a bucket, check it still holds the backup"},
		{backup.ErrOtherTeam, "backup_folder_taken", "choose another folder"},
		{backup.ErrNotBackup, "not_a_backup", "give the folder a DAWGit backup went to"},
		{backup.ErrNoRun, "no_such_run", "dawgit backup restore --preview lists the runs"},
		{backup.ErrNotEmpty, "backup_folder_not_empty", "choose an empty folder, or this team's earlier backup"},
	} {
		if errors.Is(err, c.err) {
			return &cliError{Code: c.code, Exit: exitError, Err: err, Hint: c.hint}
		}
	}
	return err
}

func backupStatus(t *teams.Team) error {
	out := backupStatusJSON{Team: t.Name, Supported: t.Remote.IsStorage(), Members: []backupMemberJSON{}}
	if b := t.Backup; b != nil {
		if d, err := backup.DestOf(b); err == nil {
			out.Kind, out.Folder = d.Kind(), d.Name()
		}
		out.Paused, out.Error = b.Paused, b.LastError
		out.LastSuccess, out.LastAttempt, out.Failing = rfc3339(b.LastSuccess), rfc3339(b.LastAttempt), backup.Failing(b)
	}
	if out.Supported {
		members, err := backup.Members(*t)
		if err != nil {
			return err
		}
		for _, m := range members {
			out.Members = append(out.Members, backupMemberJSON{Name: m.Name, LastSuccess: rfc3339(m.LastSuccess), Failing: m.Failing})
		}
		out.Covered = backup.Covered(members)
	}
	result("backup", out, func() {
		fmt.Printf("team %q\n", t.Name)
		if !out.Supported {
			fmt.Println("only teams that keep their work in storage (R2, S3) can be backed up")
			return
		}
		switch b := t.Backup; {
		case b == nil:
			fmt.Println("this computer: no backup (set one up in the app, or: dawgit backup run <folder>)")
		default:
			state := "never backed up"
			if !b.LastSuccess.IsZero() {
				state = "last backup " + b.LastSuccess.Local().Format("2006-01-02 15:04")
			}
			if b.Paused {
				state += ", paused"
			}
			fmt.Printf("this computer: %s (%s)\n", out.Folder, state)
			if b.LastError != "" {
				fmt.Printf("  last try failed: %s\n", b.LastError)
			}
		}
		if len(out.Members) == 0 {
			fmt.Println("nobody backs up this team yet")
		}
		for _, m := range out.Members {
			fmt.Printf("  %-20s last backup %s%s\n", m.Name, orNever(m.LastSuccess), map[bool]string{true: " (failing)"}[m.Failing])
		}
	})
	return nil
}

func orNever(s string) string {
	if s == "" {
		return "never"
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return s
}

type restoreJSON struct {
	Team     string               `json:"team"`
	From     string               `json:"from"`      // the backup: a folder, or a bucket's address
	BackupOf string               `json:"backup_of"` // the team's name in the backup
	Run      string               `json:"run"`       // "" for the latest
	Runs     []string             `json:"runs"`      // newest first (UTC, 20061002-150405)
	Projects []backup.PlanProject `json:"projects"`  // brought back whole
	Branches int                  `json:"branches"`  // brought back in projects the team still has
	Files    int                  `json:"files"`     // to copy
	Bytes    int64                `json:"bytes"`     //
	Restored *int                 `json:"restored"`  // files copied; null with --preview
}

// backupRestore: bring back from a backup (the folder given, a bucket's
// connection code, or where the app backs the team up) what the team's
// storage lacks; never overwrites.
func backupRestore(t *teams.Team, pos []string, run string, preview bool) error {
	var src backup.Dest
	if len(pos) == 1 && remote.IsConnectionCode(pos[0]) {
		// A backup in S3-compatible storage (dawgit connection-code makes
		// one from its address and keys; reading it is enough).
		cfg, err := remote.ParseAddress(pos[0], "")
		if err != nil {
			return usageError("%v", err)
		}
		if backup.Overlaps(cfg, *t) {
			return usageError("that is where %q keeps its work: give where its backup is", t.Name)
		}
		if src, err = backup.Bucket(cfg); err != nil {
			return err
		}
	} else if len(pos) == 1 {
		abs, err := filepath.Abs(pos[0])
		if err != nil {
			return err
		}
		src = backup.Folder(abs)
	} else if t.Backup == nil {
		return usageError("this computer doesn't back up %q: give the backup's folder or its bucket's connection code (dawgit backup restore <folder | code>)", t.Name)
	} else {
		var err error
		if src, err = backup.DestOf(t.Backup); err != nil {
			return err
		}
	}
	s3, err := backup.Storage(*t)
	if err != nil {
		return err
	}
	p, err := backup.MakePlan(src, s3, run)
	if err != nil {
		return backupError(err)
	}
	out := restoreJSON{Team: t.Name, From: src.Name(), BackupOf: p.Team, Run: p.Run, Runs: nonNil(p.Runs),
		Projects: p.Projects, Branches: p.Branches, Files: p.Files, Bytes: p.Bytes}
	if !preview && !p.Empty() {
		var last time.Time
		rep, err := backup.Restore(src, s3, p, func(done, total int64) {
			if !jsonMode && total > 64<<20 && time.Since(last) > 2*time.Second {
				last = time.Now()
				fmt.Fprintf(os.Stderr, "  %d of %d MB\n", done>>20, total>>20)
			}
		})
		if err != nil {
			return err
		}
		out.Restored = &rep.Copied
	} else if !preview {
		zero := 0
		out.Restored = &zero
	}
	result("backup", out, func() {
		at := "the latest backup"
		if p.Run != "" {
			at = "the backup of " + p.Run + " (UTC)"
		}
		fmt.Printf("from %s, as of %s:\n", src.Name(), at)
		if p.Empty() {
			fmt.Printf("%q's storage has everything in it: nothing to restore\n", t.Name)
			return
		}
		for _, pr := range p.Projects {
			fmt.Printf("  project %-24s %d version(s)\n", pr.Name, pr.Versions)
		}
		if p.Branches > 0 {
			fmt.Printf("  %d branch(es) in projects the team still has\n", p.Branches)
		}
		verb := "would copy"
		if out.Restored != nil {
			verb = "copied"
		}
		fmt.Printf("%s %d files (%.1f MB) into %q's storage; nothing there was changed or deleted\n", verb, p.Files,
			float64(p.Bytes)/(1<<20), t.Name)
		if preview && len(p.Runs) > 0 {
			fmt.Printf("runs (for --run): %s\n", strings.Join(p.Runs, " "))
		}
	})
	return nil
}
