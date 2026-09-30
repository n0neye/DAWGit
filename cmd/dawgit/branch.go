package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"dawgit/internal/project"
)

func when(m *project.Manifest) string {
	if t, err := time.Parse(time.RFC3339, m.Time); err == nil {
		return t.Local().Format("2006-01-02 15:04")
	}
	return m.Time
}

func cmdBranch(args []string) error {
	r, err := openRepo()
	if err != nil {
		return err
	}
	if len(args) >= 2 && args[0] == "new" {
		if err := r.CreateBranch(args[1]); err != nil {
			return err
		}
		fmt.Printf("created branch %q from your current version and switched to it\n", args[1])
		fmt.Println("versions you save now go to this branch; merge back with `dawgit switch main` + `dawgit merge " + args[1] + "`")
		return nil
	}
	if len(args) > 0 {
		return errors.New("usage: dawgit branch [new NAME]")
	}
	branches, err := r.Branches()
	if err != nil {
		return err
	}
	for _, b := range branches {
		mark := "  "
		if b.Current {
			mark = "* "
		}
		latest := "(no versions yet)"
		if b.Latest != nil {
			latest = fmt.Sprintf("%s  %s  %-10s %s", short(b.Head), when(b.Latest), b.Latest.Author, b.Latest.Message)
		}
		fmt.Printf("%s%-16s %s\n", mark, b.Name, latest)
	}
	return nil
}

func cmdSwitch(args []string) error {
	fs := flag.NewFlagSet("switch", flag.ContinueOnError)
	force := fs.Bool("force", false, "discard unsaved changes and unshared versions; ignore a running Live")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: dawgit switch <branch> [--force]")
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if err := guardLiveAlways(r, *force); err != nil {
		return err
	}
	res, err := r.SwitchBranch(pos[0], *force)
	if err != nil {
		return err
	}
	fmt.Printf("now on branch %q at %s\n", pos[0], short(res.To))
	printMerge(res)
	return nil
}

func printPreview(p *project.Preview, what string) {
	switch p.Action {
	case "up-to-date":
		fmt.Println("nothing new")
		return
	case "ahead":
		fmt.Println("nothing new: you already have all of " + what)
		return
	}
	fmt.Printf("%d new version(s) from %s:\n", len(p.Versions), what)
	for _, m := range p.Versions {
		fmt.Printf("  %s  %s  %-10s %s\n", short(m.ID), when(m), m.Author, m.Message)
	}
	fmt.Println("\nchanges:")
	sym := map[string]string{"added": "+", "modified": "~", "deleted": "-"}
	for _, c := range p.Changes {
		fmt.Printf("%s %s\n", sym[c.Status], c.Path)
		if c.SetDiff != nil {
			for _, line := range strings.Split(c.SetDiff.Render(), "\n") {
				fmt.Println("    " + line)
			}
		}
	}
	if len(p.Conflicts) > 0 {
		fmt.Printf("\n%d conflict(s) with your versions (you will choose with --strategy ours|theirs|both):\n", len(p.Conflicts))
		for _, c := range p.Conflicts {
			fmt.Println("  ! " + c.String())
		}
	} else if p.Action == "merge" {
		fmt.Println("\nno conflicts: merges automatically")
	}
}

func cmdMergeBranch(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	preview := fs.Bool("preview", false, "show what would come in, change nothing")
	strategy := strategyFlag(fs)
	force := fs.Bool("force", false, "merge even while Ableton Live is running")
	message := fs.String("m", "", `describe the merge version (default "Merge branch <branch>")`)
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: dawgit merge <branch> [-m message] [--preview] [--strategy ...]")
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	if *preview {
		p, err := r.PreviewMerge(pos[0])
		if err != nil {
			return err
		}
		printPreview(p, "branch "+pos[0])
		return nil
	}
	if err := guardLiveAlways(r, *force); err != nil {
		return err
	}
	res, err := r.MergeBranch(pos[0], *message, project.Strategy(*strategy))
	if err != nil {
		return explainConflict(err)
	}
	switch res.Action {
	case "up-to-date", "ahead":
		fmt.Printf("nothing to merge: you already have everything from %s\n", pos[0])
	default:
		printMerge(res)
		fmt.Printf("merged %s into %s and shared it (%s); reopen the set in Live\n", pos[0], r.BranchName(), short(res.To))
	}
	return nil
}

// guardLiveAlways refuses while a set of the project is open in Live: the
// command rewrites sets.
func guardLiveAlways(r *project.Repo, force bool) error {
	if force {
		return nil
	}
	if set := openSet(r.Root); set != "" {
		return liveOpenError(set, "")
	}
	return nil
}
