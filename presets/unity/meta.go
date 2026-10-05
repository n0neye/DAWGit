package unity

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/ext"
)

// checkMeta warns when an asset and its .meta don't travel together: Unity
// refers to assets by the GUID in the .meta, so a missing or orphaned .meta
// breaks references on the other computers. It looks at the whole project
// (changes and files committed before, "unchanged"): an asset committed
// without its .meta is warned about at every commit until it has one.
func checkMeta(root string, changes []ext.Change) []string {
	isDir := func(rel string) bool {
		fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil && fi.IsDir()
	}
	status := map[string]string{}
	for _, c := range changes {
		status[c.Path] = c.Status
	}
	// What the commit leaves: files, and the folders they are in.
	there := map[string]bool{}
	for p, s := range status {
		if s == "deleted" || s == "untracked" { // untracked: no longer committed
			continue
		}
		there[p] = true
		for d := path.Dir(p); d != "." && !there[d]; d = path.Dir(d) {
			there[d] = true
		}
	}
	var out []string
	paths := make([]string, 0, len(status))
	for p := range status {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		s := status[p]
		if asset, ok := strings.CutSuffix(p, ".meta"); ok {
			if !imported(asset) {
				continue
			}
			switch {
			case s == "deleted" && there[asset]:
				out = append(out, fmt.Sprintf("%s is deleted but %s is still there: Unity will give it a new GUID", p, asset))
			case s == "added" && !there[asset] && !isDir(asset):
				// An empty folder keeps its .meta: it's there, just not committed.
				out = append(out, fmt.Sprintf("%s has no asset next to it (delete the .meta, or add %s)", p, path.Base(asset)))
			}
			continue
		}
		if !imported(p) {
			continue
		}
		meta := p + ".meta"
		switch {
		case s == "untracked":
		case s == "deleted":
			if there[meta] && status[meta] != "deleted" {
				out = append(out, fmt.Sprintf("%s is deleted but its .meta stays", p))
			}
		case there[meta], status[meta] == "deleted": // that one says so
		case s == "added":
			out = append(out, fmt.Sprintf("%s has no .meta yet: open the project in Unity so it imports the file", p))
		default:
			out = append(out, fmt.Sprintf("%s has no .meta: open the project in Unity so it makes one, and commit it", p))
		}
	}
	// Folders need a .meta too (Unity gives each a GUID).
	var folders []string
	for d := range there {
		if _, isFile := status[d]; !isFile && imported(d) && !there[d+".meta"] {
			folders = append(folders, d)
		}
	}
	sort.Strings(folders)
	for _, d := range folders {
		out = append(out, fmt.Sprintf("the folder %s has no .meta: open the project in Unity so it makes one, and commit it", d))
	}
	return out
}

// imported reports whether Unity imports the file or folder at rel (and so
// gives it a .meta): under Assets/, or inside a package in Packages/, and
// not hidden from the import. Unity skips names starting with "." or
// ending with "~", "cvs", and *.tmp files, and everything inside such
// folders (Samples~, Documentation~).
func imported(rel string) bool {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) >= 2 && parts[0] == "Assets":
		parts = parts[1:]
	case len(parts) >= 3 && parts[0] == "Packages": // Packages/<package>/…
		parts = parts[2:]
	default:
		return false
	}
	for _, name := range parts {
		lower := strings.ToLower(name)
		if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
			lower == "cvs" || strings.HasSuffix(lower, ".tmp") {
			return false
		}
	}
	return true
}
