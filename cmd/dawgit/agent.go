package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"dawgit/internal/agent"
	"dawgit/internal/project"
)

// editLabel is "Song.als: Bass" (or a set-wide change like "tempo: 120 -> 128").
func editLabel(e project.TrackEdit) string { return agent.Label(e) }

func logf(format string, a ...any) {
	fmt.Printf("%s  %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func printEvent(e agent.Event) {
	switch e.Kind {
	case agent.BackedUp:
		logf("backed up your unsaved work: %s", strings.Join(e.Labels, ", "))
	case agent.NewVersions:
		for _, m := range e.Versions {
			logf("%s saved a new version: %q", m.Author, m.Message)
		}
		if e.Waiting {
			logf("(these were already waiting)")
		}
		logf("run `dawgit update --preview` to see the changes, `dawgit update` to get them")
	case agent.TeammateEditing:
		logf("%s is editing: %s", e.Author, strings.Join(e.Labels, ", "))
	case agent.TeammateIdle:
		logf("%s has no unsaved work any more", e.Author)
	case agent.Overlap:
		logf("heads up: %s", e.Text)
	case agent.Offline:
		logf("cannot reach the server (%s); will keep trying", e.Text)
	case agent.Online:
		logf("server reachable again")
	}
}

func cmdAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	interval := fs.Duration("interval", 0, "how often to check (default: 5s for a server, 20s for storage)")
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
	if *interval == 0 {
		*interval = r.PollInterval()
	}
	agent.Run(ctx, r.Root, *interval, printEvent)
	fmt.Println("stopped")
	return nil
}
