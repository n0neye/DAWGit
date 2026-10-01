// Package cli is DAWGit's command line tool, as a library: cmd/dawgit runs it,
// and so can a build with extensions (see dawgit/ext).
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dawgit/internal/als"
	"dawgit/internal/diff"
	"dawgit/internal/merge"
	"dawgit/internal/version"
)

const usage = `usage: dawgit <command> [args]

everyday (run inside an Ableton project folder):
  save -m MESSAGE                        save a version and share it with the team
  update [--preview]                     get the team's latest versions (or just look)
  status                                 what changed since your last version
  agent                                  keep running: tell you about new versions (never changes
                                         your files)
  log                                    list versions

setup:
  serve [--data DIR] [--addr :7331] [--name TEAM]   run the team server
  init [--author NAME]                   start tracking this project
  remote <url> --token TOKEN             connect this project to the team server
  remote <connection-code>               ...or to team storage (S3-compatible bucket)
  clone <url|code> <project> [folder] [--token TOKEN]
  teams                                  teams this computer is connected to
  connection-code --endpoint URL --bucket NAME --access-key K --secret-key S [--prefix P] [--name TEAM]
                                         create a code for team storage

branches (advanced):
  branch                                 list branches
  branch new NAME                        start a branch from your current version
  switch NAME                            work on another branch
  merge NAME [--preview]                 merge another branch into yours (or just look)

advanced:
  snapshot -m MESSAGE                    save a version locally only
  checkout <id|HEAD~N|latest> [--force]  go to a version (files and samples); latest goes back
  export <id> <folder>                   write a version as a separate project folder
  profile check                          the project's rules (.dawgit.yaml) and whether they work
  profile explain <file>...              why a file is tracked or ignored
  gc                                     free space in .dawgit (files the team's storage has,
                                         leftovers no version uses)
  verify [--repair]                      check the history: every version and stored file;
                                         --repair brings back what it can
  storage-cleanup [--delete]             files in the team's storage no version uses; --delete
                                         deletes those unused for a day (and a week old)

set commands:
  info <set.als>                         tracks, devices, clips, plugins, samples
  diff <a.als> <b.als>                   semantic diff
  merge-sets <base> <ours> <theirs> -o <out>  track-level 3-way merge of files
        [--strategy fail|ours|theirs|both]

  version                                show the DAWGit version
`

// Main runs the dawgit command line tool with os.Args (extensions register
// what they add first; see dawgit/ext).
func Main() {
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
	case "merge-sets":
		code, err = cmdMerge(os.Args[2:])
	case "merge":
		err = cmdMergeBranch(os.Args[2:])
	case "branch":
		err = cmdBranch(os.Args[2:])
	case "switch":
		err = cmdSwitch(os.Args[2:])
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
	case "export":
		err = cmdExport(os.Args[2:])
	case "gc":
		err = cmdGC()
	case "verify":
		code, err = cmdVerify(os.Args[2:])
	case "storage-cleanup":
		err = cmdStorageCleanup(os.Args[2:])
	case "profile":
		err = cmdProfile(os.Args[2:])
	case "serve":
		err = cmdServe(os.Args[2:])
	case "remote":
		err = cmdRemote(os.Args[2:])
	case "teams":
		err = cmdTeams(os.Args[2:])
	case "connection-code":
		err = cmdConnectionCode(os.Args[2:])
	case "clone":
		err = cmdClone(os.Args[2:])
	case "agent":
		err = cmdAgent(os.Args[2:])
	case "save":
		err = cmdSave(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	case "version", "--version", "-v":
		fmt.Println("dawgit " + version.Display())
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
		return 2, fmt.Errorf("usage: dawgit merge-sets <base> <ours> <theirs> -o <out> [--strategy ...]")
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
