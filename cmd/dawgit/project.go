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
	fmt.Println(`next: dawgit snapshot -m "first version"`)
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
	changes, err := r.Status()
	if err != nil {
		return err
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
	for _, m := range log {
		when := m.Time
		if t, err := time.Parse(time.RFC3339, m.Time); err == nil {
			when = t.Local().Format("2006-01-02 15:04")
		}
		fmt.Printf("%s  %s  %-12s %s\n", short(m.ID), when, m.Author, m.Message)
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
