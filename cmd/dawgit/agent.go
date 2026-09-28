package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dawgit/internal/project"
)

// setSignature changes whenever a set in the project root is saved or the
// workspace moves to another version.
func setSignature(r *project.Repo) string {
	sets, _ := filepath.Glob(filepath.Join(r.Root, "*.als"))
	sort.Strings(sets)
	var b strings.Builder
	b.WriteString(r.Head())
	for _, s := range sets {
		if fi, err := os.Stat(s); err == nil {
			fmt.Fprintf(&b, "|%s:%d:%d", filepath.Base(s), fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}

// editLabel is "Song.als: Bass" (or a set-wide change like "tempo: 120 -> 128").
func editLabel(e project.TrackEdit) string {
	return e.Set + ": " + e.Name
}

func logf(format string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func cmdAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	interval := fs.Duration("interval", 5*time.Second, "how often to check")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if _, err := r.Client(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("watching %q (branch %s); Ctrl+C to stop\n", r.Config.Name, r.BranchName())
	fmt.Println("your unsaved work is backed up and shown to the team; nothing here changes your files")

	var (
		lastSig       string
		lastReport    time.Time
		myEdits       []project.TrackEdit
		notified      = map[string]bool{}
		mateEditing   = map[string]map[string]bool{} // author -> labels
		warned        = map[string]bool{}
		offline       bool
		firstIncoming = true
	)
	tick := time.NewTicker(*interval)
	defer tick.Stop()
	for {
		// Re-read config: `dawgit switch` may have changed the branch.
		if fresh, err := project.Open(r.Root); err == nil {
			r = fresh
		}

		// 1. Report unsaved work when a set was saved in Live (or hourly).
		if sig := setSignature(r); sig != lastSig || time.Since(lastReport) > time.Hour {
			st, err := r.ReportWorkspace()
			if err == nil {
				lastSig, lastReport, myEdits = sig, time.Now(), st.Edits
				if len(st.Edits) > 0 {
					var names []string
					for _, e := range st.Edits {
						names = append(names, editLabel(e))
					}
					logf("backed up your unsaved work: %s", strings.Join(names, ", "))
				}
			}
			offline = reportOffline(err, offline)
		}

		// 2. New versions from the team: notify only.
		if incoming, err := r.IncomingVersions(); err == nil {
			var fresh []*project.Manifest
			for _, m := range incoming {
				if !notified[m.ID] {
					notified[m.ID] = true
					fresh = append(fresh, m)
				}
			}
			if len(fresh) > 0 {
				for _, m := range fresh {
					logf("%s saved a new version: %q", m.Author, m.Message)
				}
				if firstIncoming {
					logf("(these were already waiting)")
				}
				logf("run `dawgit update --preview` to see the changes, `dawgit update` to get them")
			}
			firstIncoming = false
		}

		// 3. What teammates are editing (soft locks) and overlaps with you.
		if mates, err := r.Teammates(); err == nil {
			seen := map[string]bool{}
			for _, w := range mates {
				seen[w.Author] = true
				now := map[string]bool{}
				for _, e := range w.Edits {
					now[editLabel(e)] = true
				}
				var started []string
				for l := range now {
					if !mateEditing[w.Author][l] {
						started = append(started, l)
					}
				}
				sort.Strings(started)
				if len(started) > 0 {
					logf("%s is editing: %s", w.Author, strings.Join(started, ", "))
				}
				mateEditing[w.Author] = now
			}
			for author := range mateEditing {
				if !seen[author] {
					logf("%s has no unsaved work any more", author)
					delete(mateEditing, author)
				}
			}
			for _, o := range project.Overlaps(myEdits, mates) {
				if !warned[o] {
					warned[o] = true
					logf("heads up: %s", o)
				}
			}
		}

		select {
		case <-ctx.Done():
			fmt.Println("stopped")
			return nil
		case <-tick.C:
		}
	}
}

func reportOffline(err error, wasOffline bool) bool {
	switch {
	case err != nil && !wasOffline:
		logf("cannot reach the server (%v); will keep trying", err)
		return true
	case err == nil && wasOffline:
		logf("server reachable again")
	}
	return err != nil
}
