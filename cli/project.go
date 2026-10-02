package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"dawgit/internal/project"
	"dawgit/internal/remote"
)

// openRepo opens the project here, kept from other programs (the app) until
// the command ends.
func openRepo() (*project.Repo, error) {
	r, err := project.Open(".")
	if err != nil {
		return nil, err
	}
	if _, err := r.Lock(10 * time.Second); err != nil {
		return nil, err
	}
	return r, nil
}

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
	if head := r.Head(); r.OnOlderVersion() {
		fmt.Printf("on an older version %s (latest: %s; `dawgit checkout latest` goes back)\n", short(head), short(r.Latest()))
	} else if head != "" {
		fmt.Printf("on version %s\n", short(head))
	} else {
		fmt.Println("no snapshots yet")
	}
	if r.Config.Remote != nil {
		switch incoming, err := r.Incoming(); {
		case err != nil:
			fmt.Printf("team: not reachable (%v)\n", err)
		case incoming:
			fmt.Println("team: new versions saved by others (run `dawgit update`)")
		default:
			fmt.Println("team: up to date")
		}
	}
	changes, err := r.Status()
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Println("nothing changed")
		return nil
	}
	sym := map[string]string{"added": "+", "modified": "~", "deleted": "-", "renamed": "M", "untracked": "o"}
	for _, c := range changes {
		if c.Status == "renamed" {
			edited := ""
			if c.Edited {
				edited = " (and changed)"
			}
			fmt.Printf("M %s -> %s%s\n", c.From, c.Path, edited)
			continue
		}
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
	names := r.MemberNames()
	for _, m := range log {
		mark := ""
		if names := tips[m.ID]; len(names) > 0 {
			mark = "  [" + strings.Join(names, ", ") + "]"
		}
		fmt.Printf("%s  %s  %-12s %s%s\n", short(m.ID), when(m), project.AuthorName(m, names), m.Message, mark)
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
	r, err := openRepo()
	if err != nil {
		return err
	}
	if err := guardLiveAlways(r, *force); err != nil {
		return err
	}
	m, notes, err := r.GoTo(ref, *force)
	if err != nil {
		return err
	}
	fmt.Printf("now on version %s  %s\n", short(m.ID), m.Message)
	for _, n := range notes {
		fmt.Println("  relinked " + n)
	}
	if r.OnOlderVersion() {
		fmt.Println("this is an older version: `dawgit checkout latest` goes back, `dawgit branch new NAME` continues from here")
	}
	return nil
}

// cmdExport writes a version as a separate project folder.
func cmdExport(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: dawgit export <id|HEAD~N> <folder>")
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	m, err := r.Export(args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Printf("exported version %s  %s\n  to %s\n", short(m.ID), m.Message, args[1])
	return nil
}

// cmdGC frees space in .dawgit: for a team project, local copies of files
// the team's storage has (sets stay); for any project, objects no version
// uses.
func cmdVerify(args []string) (int, error) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	repair := fs.Bool("repair", false, "bring back what can be: from the project folder or the team's storage")
	if err := fs.Parse(args); err != nil {
		return 0, err
	}
	r, err := openRepo()
	if err != nil {
		return 0, err
	}
	rep, err := r.Verify(*repair)
	if err != nil {
		return 0, err
	}
	open := 0
	for _, p := range rep.Problems {
		what := p.ID[:min(10, len(p.ID))]
		if p.Path != "" {
			what = p.Path
		}
		status := ""
		switch {
		case p.Fixed:
			status = "  -> repaired: " + p.How
		case p.How != "":
			status = "  -> " + p.How
			open++
		default:
			open++
		}
		fmt.Printf("%-12s %s: %s%s\n", p.Kind, what, p.Detail, status)
	}
	if r.Config.Remote != nil && !rep.TeamChecked {
		fmt.Println("(the team's storage couldn't be reached: files only it has weren't checked)")
	}
	fmt.Println(rep.Summary())
	if open > 0 {
		if !*repair {
			fmt.Println("run `dawgit verify --repair` to bring back what can be")
		}
		return 1, nil
	}
	return 0, nil
}

func cmdStorageCleanup(args []string) error {
	fs := flag.NewFlagSet("storage-cleanup", flag.ContinueOnError)
	del := fs.Bool("delete", false, "delete the unused files that are due")
	if err := fs.Parse(args); err != nil {
		return err
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	c, err := r.Client()
	if err != nil {
		return err
	}
	s3, ok := c.(*remote.S3Backend)
	if !ok {
		return errors.New("cleanup works on teams that use S3 or R2 storage")
	}
	rep, err := s3.CollectGarbage(*del)
	if err != nil {
		return err
	}
	mb := func(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }
	fmt.Printf("%d files in storage, used by %d versions\n", rep.Stored, rep.Versions)
	if rep.Deleted > 0 {
		fmt.Printf("deleted %d unused files (%s)\n", rep.Deleted, mb(rep.DeletedBytes))
	}
	switch {
	case rep.Waiting == 0:
		fmt.Println("nothing (else) unused")
	case rep.Due > rep.Deleted:
		fmt.Printf("%d unused files (%s), %d (%s) due: run with --delete\n", rep.Waiting, mb(rep.WaitingBytes),
			rep.Due, mb(rep.DueBytes))
	default:
		fmt.Printf("%d unused files (%s) can be deleted from %s\n", rep.Waiting, mb(rep.WaitingBytes),
			rep.NextCleanup.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func cmdGC() error {
	r, err := openRepo()
	if err != nil {
		return err
	}
	pruned, err := r.PruneObjects()
	if err != nil {
		return err
	}
	freed, err := r.GC()
	if err != nil {
		return err
	}
	fmt.Printf("%.1f MB kept in the team's storage only, %.1f MB of leftovers removed\n",
		float64(pruned)/(1<<20), float64(freed)/(1<<20))
	return nil
}
