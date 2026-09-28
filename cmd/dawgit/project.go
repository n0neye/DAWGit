package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"dawgit/internal/livecheck"
	"dawgit/internal/project"
)

func openRepo() (*project.Repo, error) { return project.Open(".") }

func short(id string) string { return id[:min(10, len(id))] }

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	author := fs.String("author", "", "name recorded on snapshots (default: OS user)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	r, err := project.Init(dir, *author)
	if err != nil {
		return err
	}
	fmt.Printf("initialized dawgit project in %s (author %s)\n", r.Root, r.Config.Author)
	fmt.Println("next: dawgit remote <server-url> --token TOKEN, then dawgit save -m \"first version\"")
	return nil
}

func cmdStatus(args []string) error {
	r, err := openRepo()
	if err != nil {
		return err
	}
	if head := r.Head(); head != "" {
		fmt.Printf("on snapshot %s\n", short(head))
	} else {
		fmt.Println("no snapshots yet")
	}
	if r.Config.Remote != nil {
		switch incoming, err := r.Incoming(); {
		case err != nil:
			fmt.Printf("server: not reachable (%v)\n", err)
		case incoming:
			fmt.Println("server: the team saved new versions (run `dawgit update`)")
		default:
			fmt.Println("server: up to date")
		}
	}
	changes, err := r.Status()
	if err != nil {
		return err
	}
	if r.Config.Remote != nil {
		printTeammates(r)
	}
	if len(changes) == 0 {
		fmt.Println("nothing changed")
		return nil
	}
	sym := map[string]string{"added": "+", "modified": "~", "deleted": "-"}
	for _, c := range changes {
		fmt.Printf("%s %s\n", sym[c.Status], c.Path)
		if c.SetDiff != nil {
			for _, line := range strings.Split(c.SetDiff.Render(), "\n") {
				fmt.Println("    " + line)
			}
		}
	}
	return nil
}

func cmdSnapshot(args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	msg := fs.String("m", "", "snapshot message")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *msg == "" {
		return errors.New(`a message is required: dawgit snapshot -m "what changed"`)
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	m, err := r.Snapshot(*msg)
	if errors.Is(err, project.ErrNothingToSnapshot) {
		fmt.Println(err)
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("snapshot %s  %s\n", short(m.ID), m.Message)
	fmt.Printf("  %d file(s), %d external sample(s)\n", len(m.Files), len(m.External))
	if len(m.Packs) > 0 {
		fmt.Printf("  uses Live packs: %s\n", strings.Join(m.Packs, ", "))
	}
	for _, p := range m.Missing {
		fmt.Printf("  warning: referenced sample not found: %s\n", p)
	}
	return nil
}

func cmdLog(args []string) error {
	r, err := openRepo()
	if err != nil {
		return err
	}
	log, err := r.Log()
	if err != nil {
		return err
	}
	if len(log) == 0 {
		fmt.Println("no snapshots yet")
	}
	// Mark where each server branch is (best effort).
	tips := map[string][]string{}
	if r.Config.Remote != nil {
		if branches, err := r.Branches(); err == nil {
			for _, b := range branches {
				tips[b.Head] = append(tips[b.Head], b.Name)
			}
		}
	}
	for _, m := range log {
		mark := ""
		if names := tips[m.ID]; len(names) > 0 {
			mark = "  [" + strings.Join(names, ", ") + "]"
		}
		fmt.Printf("%s  %s  %-12s %s%s\n", short(m.ID), when(m), m.Author, m.Message, mark)
	}
	return nil
}

func cmdCheckout(args []string) error {
	fs := flag.NewFlagSet("checkout", flag.ContinueOnError)
	force := fs.Bool("force", false, "discard changes that are not snapshotted; ignore a running Live")
	var ref string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) > 0 {
			ref, args = args[0], args[1:]
		}
	}
	if ref == "" {
		return errors.New("usage: dawgit checkout <id|HEAD> [--force]")
	}
	if livecheck.Running() && !*force {
		return errors.New("Ableton Live is running; close the set first (Live would overwrite the restored files on save), or use --force")
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	m, notes, err := r.Checkout(ref, *force)
	if err != nil {
		return err
	}
	fmt.Printf("now on snapshot %s  %s\n", short(m.ID), m.Message)
	for _, n := range notes {
		fmt.Println("  relinked " + n)
	}
	return nil
}

// printTeammates shows what others are editing (soft locks) and overlaps.
func printTeammates(r *project.Repo) {
	mates, err := r.Teammates()
	if err != nil || len(mates) == 0 {
		return
	}
	fmt.Println("team members editing (not saved yet):")
	for _, w := range mates {
		var names []string
		for _, e := range w.Edits {
			names = append(names, editLabel(e))
		}
		age := time.Since(w.UpdatedTime()).Round(time.Minute)
		fmt.Printf("  %s (%s ago): %s\n", w.Author, age, strings.Join(names, ", "))
	}
	if mine, _, err := r.LocalEdits(); err == nil {
		for _, o := range project.Overlaps(mine, mates) {
			fmt.Println("  heads up: " + o)
		}
	}
}
