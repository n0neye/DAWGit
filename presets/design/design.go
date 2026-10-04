// Package design teaches DAWGit about design, 3D and video projects:
// Photoshop, Affinity, Blender, Cinema 4D, Maya, DaVinci Resolve and
// Resolume files, often several kinds in one folder. Their backups and
// caches are left out, and files are not rewritten while one of these
// programs has them open. Importing it is enough.
package design

import (
	_ "embed"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dawgit/ext"
	"dawgit/internal/wintitle"
)

//go:embed design.yaml
var preset []byte

func init() {
	if err := ext.RegisterPreset(preset); err != nil {
		panic("design preset: " + err.Error())
	}
	ext.RegisterRunning("design-apps", openIn)
}

// apps are the programs whose window titles name the file they have open:
// "hero.psd @ 50% (RGB/8)", "Blender [D:\Art\scene.blend]", "Autodesk Maya
// 2025: D:\Art\rig.mb", "Cinema 4D 2025 - [city.c4d]", "Resolume Arena -
// show". (DaVinci Resolve keeps its projects in its own database.)
var apps = []struct{ exe, name string }{
	{"Photoshop.exe", "Photoshop"},
	{"Photo.exe", "Affinity Photo"}, {"Designer.exe", "Affinity Designer"}, {"Publisher.exe", "Affinity Publisher"},
	{"Affinity.exe", "Affinity"},
	{"blender.exe", "Blender"},
	{"Cinema 4D.exe", "Cinema 4D"},
	{"maya.exe", "Maya"},
	{"Arena.exe", "Resolume Arena"}, {"Avenue.exe", "Resolume Avenue"},
}

var designExts = map[string]bool{".psd": true, ".psb": true, ".af": true, ".afphoto": true, ".afdesign": true, ".afpub": true,
	".blend": true, ".c4d": true, ".ma": true, ".mb": true, ".avc": true}

// openIn reports "hero.psd in Photoshop" when one of these programs has a
// file of this project open ("" when none does).
var openIn = func(root string) string {
	type window struct{ app, title string }
	var windows []window
	for _, a := range apps {
		for _, t := range wintitle.Of(a.exe) {
			windows = append(windows, window{a.name, strings.ToLower(t)})
		}
	}
	if len(windows) == 0 {
		return ""
	}
	for _, name := range projectNames(root) {
		bare := strings.TrimSuffix(name, filepath.Ext(name))
		for _, w := range windows {
			// Resolume shows a composition's name without .avc.
			if strings.Contains(w.title, name) || (strings.HasSuffix(name, ".avc") && len(bare) > 2 && strings.Contains(w.title, bare)) {
				return name + " in " + w.app
			}
		}
	}
	return ""
}

// projectNames lists (lowercased) the names of the project's files these
// programs open, a moment old at most.
func projectNames(root string) []string {
	names.Lock()
	defer names.Unlock()
	if c, ok := names.byRoot[root]; ok && time.Since(c.at) < 30*time.Second {
		return c.names
	}
	var out []string
	seen := 0
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == ".dawgit" || n == ".git" || n == "CacheClip" || n == "ProxyMedia" {
				return filepath.SkipDir
			}
			return nil
		}
		if seen++; seen > 100_000 {
			return filepath.SkipAll
		}
		if n := strings.ToLower(d.Name()); designExts[filepath.Ext(n)] {
			out = append(out, n)
		}
		return nil
	})
	names.byRoot[root] = cachedNames{out, time.Now()}
	return out
}

type cachedNames struct {
	names []string
	at    time.Time
}

var names = struct {
	sync.Mutex
	byRoot map[string]cachedNames
}{byRoot: map[string]cachedNames{}}
