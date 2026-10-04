package unity

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dawgit/ext"
)

// checkMeta warns when an asset and its .meta don't travel together: Unity
// refers to assets by the GUID in the .meta, so a missing or orphaned .meta
// breaks references on the other computers.
func checkMeta(root string, changes []ext.Change) []string {
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	changed := map[string]string{}
	for _, c := range changes {
		changed[c.Path] = c.Status
	}
	var out []string
	for _, c := range changes {
		if !strings.HasPrefix(c.Path, "Assets/") {
			continue
		}
		if asset, ok := strings.CutSuffix(c.Path, ".meta"); ok {
			switch {
			case c.Status == "added" && !exists(asset):
				out = append(out, fmt.Sprintf("%s has no asset next to it (delete the .meta, or add %s)", c.Path, path.Base(asset)))
			case c.Status == "deleted" && exists(asset):
				out = append(out, fmt.Sprintf("%s is deleted but %s is still there: Unity will give it a new GUID", c.Path, asset))
			}
			continue
		}
		meta := c.Path + ".meta"
		switch {
		case c.Status == "added" && !exists(meta):
			out = append(out, fmt.Sprintf("%s has no .meta yet: open the project in Unity so it imports the file", c.Path))
		case c.Status == "deleted" && exists(meta) && changed[meta] != "deleted":
			out = append(out, fmt.Sprintf("%s is deleted but its .meta stays", c.Path))
		}
	}
	return out
}
