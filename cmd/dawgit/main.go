// Command dawgit is version control for Ableton Live projects.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dawgit/internal/als"
	"dawgit/internal/diff"
	"dawgit/internal/merge"
)

const usage = `usage: dawgit <command> [args]

everyday (run inside an Ableton project folder):
  save -m MESSAGE                        save a version and share it with the team
  update                                 get the team's latest versions
  status                                 what changed since your last version
  log                                    list versions

setup:
  serve [--data DIR] [--addr :7331]      run the team server
  init [--author NAME]                   start tracking this project
  remote <url> --token TOKEN             connect this project to the team server
  clone <url> <project> [folder] --token TOKEN

advanced:
  snapshot -m MESSAGE                    save a version locally only
  checkout <id|HEAD~N> [--force]         restore a version and relink samples

set commands:
  info <set.als>                         tracks, devices, clips, plugins, samples
  diff <a.als> <b.als>                   semantic diff
  merge <base> <ours> <theirs> -o <out>  track-level 3-way merge
        [--strategy fail|ours|theirs|both]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	code := 0
	switch os.Args[1] {
	case "info":
		err = cmdInfo(os.Args[2:])
	case "diff":
		err = cmdDiff(os.Args[2:])
	case "merge":
		code, err = cmdMerge(os.Args[2:])
	case "init":
		err = cmdInit(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "snapshot":
		err = cmdSnapshot(os.Args[2:])
	case "log":
		err = cmdLog(os.Args[2:])
	case "checkout":
		err = cmdCheckout(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "remote":
		err = cmdRemote(os.Args[2:])
	case "clone":
		err = cmdClone(os.Args[2:])
	case "save":
		err = cmdSave(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func cmdInfo(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: dawgit info <set.als>")
	}
	s, err := als.Load(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s  [%s]  tempo %s  scenes %d  NextPointeeId %d\n",
		filepath.Base(s.Path), s.Creator(), s.Tempo(), s.SceneCount(), s.NextPointeeID())
	fmt.Println("\nTracks:")
	for _, t := range s.Tracks() {
		group := ""
		if t.GroupID() != "-1" {
			group = "  (in group " + t.GroupID() + ")"
		}
		fmt.Printf("  [%3s] %-11s %s%s\n", t.ID(), t.Kind(), t.Name(), group)
		for _, d := range t.DeviceNames() {
			fmt.Printf("          device  %s\n", d)
		}
		for _, c := range t.Clips() {
			fmt.Printf("          clip    %-9s \"%s\" %s %g-%g\n", c.Kind, c.Name, c.Location, c.Start, c.End)
		}
		var auto []string
		for label := range diff.Envelopes(t.Elem) {
			auto = append(auto, label)
		}
		sort.Strings(auto)
		for _, label := range auto {
			fmt.Printf("          auto    %s\n", label)
		}
	}
	fmt.Println("\nPlugins:")
	plugins := s.Plugins()
	if len(plugins) == 0 {
		plugins = []string{"(none)"}
	}
	for _, p := range plugins {
		fmt.Println("  " + p)
	}
	fmt.Println("\nSample references:")
	seen := map[string]bool{}
	for _, r := range s.SampleRefs() {
		key := r.RelativePath + "|" + r.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		name := r.RelativePath
		if name == "" {
			name = r.Path
		}
		pack := ""
		if r.Pack != "" {
			pack = " [" + r.Pack + "]"
		}
		fmt.Printf("  %s%s  (type %s, %s bytes)\n", name, pack, r.RelativePathType, r.FileSize)
	}
	return nil
}

func cmdDiff(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: dawgit diff <a.als> <b.als>")
	}
	a, err := als.Load(args[0])
	if err != nil {
		return err
	}
	b, err := als.Load(args[1])
	if err != nil {
		return err
	}
	fmt.Println(diff.Diff(a, b).Render())
	return nil
}

func cmdMerge(args []string) (int, error) {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	out := fs.String("o", "", "output .als path")
	strategy := fs.String("strategy", "fail", "how to resolve tracks changed on both sides: fail|ours|theirs|both")
	// Allow flags after positional args.
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return 2, err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	if len(pos) != 3 || *out == "" {
		return 2, fmt.Errorf("usage: dawgit merge <base> <ours> <theirs> -o <out> [--strategy ...]")
	}
	var sets [3]*als.LiveSet
	for i, p := range pos {
		s, err := als.Load(p)
		if err != nil {
			return 1, err
		}
		sets[i] = s
	}
	r, err := merge.Merge(sets[0], sets[1], sets[2], *strategy)
	if err != nil {
		return 2, err
	}
	fmt.Println(r.Report())
	if len(r.Conflicts) > 0 && *strategy == "fail" {
		fmt.Fprintln(os.Stderr, "\nmerge aborted: unresolved conflicts (use --strategy ours|theirs|both)")
		return 1, nil
	}
	if err := r.Merged.Save(*out); err != nil {
		return 1, err
	}
	fmt.Printf("\nwrote %s\n", *out)
	return 0, nil
}
