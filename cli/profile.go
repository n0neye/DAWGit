package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/profile"
)

// cmdProfile: `dawgit profile check` and `dawgit profile explain <file>...`.
func cmdProfile(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: dawgit profile check | dawgit profile explain <file>...")
	}
	r, err := openRepo()
	if err != nil {
		return err
	}
	rules, loadErr := r.Profile()
	switch args[0] {
	case "check":
		if rules.FromFile {
			fmt.Printf("rules: %s\n", profile.FileName)
		} else {
			fmt.Printf("rules: detected (no %s)\n", profile.FileName)
		}
		for _, a := range rules.Applied() {
			folder := a.Folder
			if folder == "" {
				folder = "(project folder)"
			}
			fmt.Printf("  %s: %s\n", folder, a.Preset)
		}
		for i, ru := range rules.Rules {
			if ru.Ignore != "" {
				fmt.Printf("  rule %d: ignore %q\n", i+1, ru.Ignore)
			} else {
				fmt.Printf("  rule %d: track %q\n", i+1, ru.Track)
			}
		}
		if err := r.CheckRules(); err != nil {
			return err
		}
		fmt.Println("ok")
		return loadErr
	case "explain":
		if len(args) < 2 {
			return errors.New("usage: dawgit profile explain <file>...")
		}
		cwd, _ := os.Getwd()
		for _, arg := range args[1:] {
			abs := arg
			if !filepath.IsAbs(abs) {
				abs = filepath.Join(cwd, arg)
			}
			rel, err := filepath.Rel(r.Root, abs)
			if err != nil || strings.HasPrefix(rel, "..") {
				return fmt.Errorf("%s is not in this project", arg)
			}
			rel = filepath.ToSlash(rel)
			fi, statErr := os.Stat(abs)
			isDir := statErr == nil && fi.IsDir()
			d := rules.Explain(rel, isDir)
			state := "tracked"
			if d.Ignored {
				state = "ignored"
			}
			fmt.Printf("%s: %s (%s)", rel, state, d.By)
			if !isDir {
				fmt.Printf(", kind %s", rules.Kind(rel))
				if h := rules.Handler(rel); h.Merge != "" || h.Samples != "" {
					fmt.Printf(", handled by %s", strings.Trim(h.Merge+" "+h.Samples, " "))
				}
			}
			fmt.Println()
		}
		return loadErr
	case "preset":
		if len(args) != 3 {
			return errors.New("usage: dawgit profile preset <folder> <preset|none>")
		}
		folder := filepath.ToSlash(filepath.Clean(args[1]))
		if err := r.SetPreset(folder, args[2]); err != nil {
			return err
		}
		fmt.Printf("%s: %s now uses %s (commit it with the project)\n", profile.FileName, args[1], args[2])
		return nil
	}
	return fmt.Errorf("unknown profile command %q (check, explain, preset)", args[0])
}
