package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dawgit path add|remove [folder]: puts the folder of this dawgit.exe (or
// the one given) on the user's PATH, or takes it off — only that entry,
// nothing else. The installer runs it (an NSIS string would cut a long PATH
// short). Not in the usage: it is the installer's.
func cmdPath(args []string) error {
	if len(args) < 1 || len(args) > 2 || (args[0] != "add" && args[0] != "remove") {
		return usageError("usage: dawgit path add|remove [folder]")
	}
	dir := ""
	if len(args) == 2 {
		dir = args[1]
	} else {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		dir = filepath.Dir(exe)
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	changed, err := userPath(args[0] == "add", dir)
	if err != nil {
		return err
	}
	switch {
	case !changed:
		fmt.Println("PATH already as it should be")
	case args[0] == "add":
		fmt.Printf("added %s to your PATH (open a new terminal to use dawgit)\n", dir)
	default:
		fmt.Printf("removed %s from your PATH\n", dir)
	}
	return nil
}

// samePath: two PATH entries name the same folder (expand turns
// %VARIABLES% into their values).
func samePath(a, b string, expand func(string) string) bool {
	norm := func(p string) string {
		p = strings.TrimSpace(strings.Trim(expand(strings.TrimSpace(p)), `"`))
		if p == "" {
			return ""
		}
		return strings.ToLower(filepath.Clean(p))
	}
	return norm(a) != "" && norm(a) == norm(b)
}

// editPath adds dir to (add) or removes it from a PATH value; changed says
// whether the value is different. Other entries stay exactly as they are.
func editPath(value, dir string, add bool, expand func(string) string) (string, bool) {
	var entries []string
	if value != "" {
		entries = strings.Split(value, ";")
	}
	var kept []string
	found := false
	for _, e := range entries {
		if samePath(e, dir, expand) {
			found = true
			if !add {
				continue
			}
		}
		kept = append(kept, e)
	}
	if add {
		if found {
			return value, false
		}
		if len(kept) > 0 && kept[len(kept)-1] == "" { // a trailing ;
			kept = kept[:len(kept)-1]
		}
		kept = append(kept, dir)
		return strings.Join(kept, ";"), true
	}
	if !found {
		return value, false
	}
	return strings.Join(kept, ";"), true
}
