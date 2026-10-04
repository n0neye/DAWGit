// Package cli is DAWGit's command line tool, as a library: cmd/dawgit runs it,
// and so can a build with extensions (see dawgit/ext).
package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dawgit/docs"
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
  watch                                  keep running: tell you about new versions (never changes
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
  profile preset <folder> <preset|none>  which preset applies to a folder (presets: in .dawgit.yaml)
  gc                                     free space in .dawgit (files the team's storage has,
                                         leftovers no version uses)
  verify [--repair]                      check the history: every version and stored file;
                                         --repair brings back what it can
  backup run [folder] [--team NAME]      back up the whole team (every project and version) into
                                         a folder; only adds. Default: the app's backup folder
  backup status [--team NAME]            this computer's backup, and who else backs up the team
  storage-cleanup [--delete]             files in the team's storage no version uses; --delete
                                         deletes those unused for a day (and a week old)

set commands:
  info <set.als>                         tracks, devices, clips, plugins, samples
  diff <a.als> <b.als>                   semantic diff
  merge-sets <base> <ours> <theirs> -o <out>  track-level 3-way merge of files
        [--strategy fail|ours|theirs|both]

  version                                show the DAWGit version

for programs and AI agents:
  --json                                 status, log, save, update, merge, backup, version: one
                                         JSON object on stdout; errors with fixed codes
  help agents [--snippet]                how AI agents use DAWGit (or lines for a project's
                                         AGENTS.md)
`

// Main runs the dawgit command line tool with os.Args (extensions register
// what they add first; see dawgit/ext).
func Main() { os.Exit(Run(os.Args[1:])) }

// Run runs one dawgit command and returns its exit status (see output.go).
func Run(args []string) int {
	args, jsonMode = stripJSON(args)
	if len(args) < 1 {
		if jsonMode {
			return fail("", usageError("usage: dawgit <command> [args] (dawgit help)"))
		}
		fmt.Fprint(os.Stderr, usage)
		return exitUsage
	}
	command, rest := args[0], args[1:]
	if jsonMode && !jsonCommands[command] {
		return fail(command, usageError("--json is not supported by `dawgit %s`", command))
	}
	defer releaseHeld()
	var err error
	code := 0
	switch command {
	case "info":
		err = cmdInfo(rest)
	case "diff":
		err = cmdDiff(rest)
	case "merge-sets":
		code, err = cmdMerge(rest)
	case "merge":
		err = cmdMergeBranch(rest)
	case "branch":
		err = cmdBranch(rest)
	case "switch":
		err = cmdSwitch(rest)
	case "init":
		err = cmdInit(rest)
	case "status":
		err = cmdStatus(rest)
	case "snapshot":
		err = cmdSnapshot(rest)
	case "log":
		err = cmdLog(rest)
	case "checkout":
		err = cmdCheckout(rest)
	case "export":
		err = cmdExport(rest)
	case "gc":
		err = cmdGC()
	case "verify":
		code, err = cmdVerify(rest)
	case "backup":
		err = cmdBackup(rest)
	case "storage-cleanup":
		err = cmdStorageCleanup(rest)
	case "profile":
		err = cmdProfile(rest)
	case "serve":
		err = cmdServe(rest)
	case "remote":
		err = cmdRemote(rest)
	case "teams":
		err = cmdTeams(rest)
	case "connection-code":
		err = cmdConnectionCode(rest)
	case "clone":
		err = cmdClone(rest)
	case "watch", "agent": // agent: its name before 0.9.8
		err = cmdWatch(rest)
	case "save":
		err = cmdSave(rest)
	case "update":
		err = cmdUpdate(rest)
	case "-h", "--help", "help":
		err = cmdHelp(rest)
	case "version", "--version", "-v":
		result("version", map[string]string{"name": version.Name(), "version": version.Version,
			"display": version.Display()}, func() { fmt.Println("dawgit " + version.Display()) })
	case "path":
		err = cmdPath(rest)
	default:
		if jsonMode {
			return fail(command, usageError("unknown command %q", command))
		}
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", command, usage)
		return exitUsage
	}
	if err != nil {
		return fail(command, err)
	}
	return code
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

// cmdHelp: dawgit help [agents [--snippet]].
func cmdHelp(args []string) error {
	switch {
	case len(args) == 0:
		fmt.Print(usage)
	case args[0] == "agents" && len(args) == 1:
		fmt.Print(docs.Agents)
	case args[0] == "agents" && len(args) == 2 && args[1] == "--snippet":
		fmt.Print(docs.AgentsSnippet)
	default:
		return usageError("usage: dawgit help [agents [--snippet]]")
	}
	return nil
}
