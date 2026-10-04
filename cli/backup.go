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
	"dawgit/internal/teams"
)

// dawgit backup: back up a team's storage into a folder (add-only, see
// internal/backup), the way the app does daily. For a scheduled task on a
// NAS or an agent: `dawgit backup run` backs up to the folder chosen in the
// app; `dawgit backup run <folder>` to any folder.

const backupUsage = "usage: dawgit backup run [folder] [--team NAME] | dawgit backup status [--team NAME]"

type backupRunJSON struct {
	Team        string `json:"team"`
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
	// This computer's backup folder (the app's), "" if none.
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
	folder, claim := "", true
	if len(pos) == 1 {
		abs, err := filepath.Abs(pos[0])
		if err != nil {
			return err
		}
		folder = abs
	} else if t.Backup != nil {
		folder = t.Backup.Folder
	} else {
		return usageError("this computer has no backup folder for %q: give one (dawgit backup run <folder>)", t.Name)
	}
	// The app's own folder must be there already (an unplugged drive).
	if t.Backup != nil && sameFolder(folder, t.Backup.Folder) {
		folder, claim = t.Backup.Folder, false
	}
	var last time.Time
	progress := func(done, total int64) {
		if !jsonMode && total > 64<<20 && time.Since(last) > 2*time.Second {
			last = time.Now()
			fmt.Fprintf(os.Stderr, "  %d of %d MB\n", done>>20, total>>20)
		}
	}
	rep, err := backup.RunTeam(t.ID, folder, claim, progress)
	if err != nil {
		return backupError(err)
	}
	out := backupRunJSON{Team: t.Name, Folder: folder, Run: rep.Run, Keys: rep.Keys, Copied: rep.Copied,
		CopiedBytes: rep.CopiedBytes, TotalBytes: rep.TotalBytes}
	result("backup", out, func() {
		fmt.Printf("backed up %q to %s: %d new files (%.1f MB); %d files (%.1f MB) in all\n", t.Name, folder,
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
		{backup.ErrMissing, "backup_folder_missing", "connect the drive (or NAS) the backup folder is on"},
		{backup.ErrOtherTeam, "backup_folder_taken", "choose another folder"},
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
		out.Folder, out.Paused, out.Error = b.Folder, b.Paused, b.LastError
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
			fmt.Println("this computer: no backup folder (set one in the app, or: dawgit backup run <folder>)")
		default:
			state := "never backed up"
			if !b.LastSuccess.IsZero() {
				state = "last backup " + b.LastSuccess.Local().Format("2006-01-02 15:04")
			}
			if b.Paused {
				state += ", paused"
			}
			fmt.Printf("this computer: %s (%s)\n", b.Folder, state)
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
