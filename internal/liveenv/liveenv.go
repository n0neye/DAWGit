// Package liveenv tells what this computer has of Ableton Live: the Live
// versions installed, the plugins Live found (from its own plugin list, with
// their versions) and the packs installed. Read only, best effort: what it
// can't read is left unknown, never guessed.
package liveenv

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"dawgit/internal/sqliteread"
)

// Install is a Live installed here.
type Install struct {
	Name    string `json:"name"`    // "Ableton Live 12 Suite"
	Version string `json:"version"` // "12.3.2" ("" when unknown)
	Edition string `json:"edition"` // "Suite", "Standard", "Intro", "Lite" ("" when unknown)
	Path    string `json:"path"`    // its folder
}

// Plugin is a plugin Live found here.
type Plugin struct {
	ID      string `json:"id"`     // Live's device id, e.g. device:vst3:instr:<uid>
	Format  string `json:"format"` // "VST3", "VST", "AU"
	Name    string `json:"name"`
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
	File    string `json:"file"` // the plugin's file
}

// UID is the plugin's id without Live's prefix: the VST3 class id, or the
// VST2 unique id.
func (p Plugin) UID() string { return p.ID[strings.LastIndex(p.ID, ":")+1:] }

// Env is what this computer has.
type Env struct {
	Installs []Install `json:"installs"`
	// Plugins Live found; PluginsKnown: Live's plugin list could be read.
	Plugins      []Plugin `json:"plugins"`
	PluginsKnown bool     `json:"pluginsKnown"`
	// Packs installed (their names); PacksKnown: Live's library settings
	// could be read.
	Packs      []string `json:"packs"`
	PacksKnown bool     `json:"packsKnown"`
}

// Dirs are where to look (Windows: %ProgramData%, %APPDATA%, %LOCALAPPDATA%).
type Dirs struct{ ProgramData, AppData, LocalAppData string }

// SystemDirs are this computer's.
func SystemDirs() Dirs {
	return Dirs{ProgramData: os.Getenv("ProgramData"), AppData: os.Getenv("APPDATA"),
		LocalAppData: os.Getenv("LOCALAPPDATA")}
}

// Read looks at this computer.
func Read() *Env { return ReadFrom(SystemDirs(), extraInstalls()) }

// ReadFrom looks in dirs; extra are more Live folders (from the system's
// list of installed programs).
func ReadFrom(d Dirs, extra []string) *Env {
	e := &Env{Installs: []Install{}, Plugins: []Plugin{}, Packs: []string{}}
	e.Installs = installs(d, extra)
	if ps, err := plugins(d); err == nil {
		e.Plugins, e.PluginsKnown = ps, true
	}
	if ps, ok := packs(d, e.Installs); ok {
		e.Packs, e.PacksKnown = ps, true
	}
	return e
}

// Newest is the newest Live installed (nil if none).
func (e *Env) Newest() *Install {
	var best *Install
	for i := range e.Installs {
		if best == nil || Compare(e.Installs[i].Version, best.Version) > 0 {
			best = &e.Installs[i]
		}
	}
	return best
}

func installs(d Dirs, extra []string) []Install {
	var dirs []string
	if d.ProgramData != "" {
		ds, _ := filepath.Glob(filepath.Join(d.ProgramData, "Ableton", "*"))
		dirs = append(dirs, ds...)
	}
	dirs = append(dirs, extra...)
	seen := map[string]bool{}
	out := []Install{}
	for _, dir := range dirs {
		if strings.HasPrefix(filepath.Base(dir), ".") { // left by an update
			continue
		}
		exes, _ := filepath.Glob(filepath.Join(dir, "Program", "Ableton Live*.exe"))
		for _, exe := range exes {
			key := strings.ToLower(filepath.Clean(exe))
			if seen[key] {
				continue
			}
			seen[key] = true
			name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
			in := Install{Name: name, Version: fileVersion(exe), Path: dir}
			if in.Version == "" { // "Ableton Live 12 Suite": at least the major version
				if f := strings.Fields(name); len(f) >= 3 {
					in.Version = f[2]
				}
			}
			var cfg struct{ Variant string }
			if data, err := os.ReadFile(filepath.Join(dir, "Program", "Installation.cfg")); err == nil &&
				json.Unmarshal(data, &cfg) == nil {
				in.Edition = cfg.Variant
			}
			if in.Edition == "" {
				for _, ed := range []string{"Suite", "Standard", "Intro", "Lite", "Trial"} {
					if strings.HasSuffix(name, " "+ed) {
						in.Edition = ed
					}
				}
			}
			out = append(out, in)
		}
	}
	sort.Slice(out, func(i, j int) bool { return Compare(out[i].Version, out[j].Version) > 0 })
	return out
}

// plugins reads Live's plugin list (the newest Live-plugins-*.db).
func plugins(d Dirs) ([]Plugin, error) {
	dbs, _ := filepath.Glob(filepath.Join(d.LocalAppData, "Ableton", "Live Database", "Live-plugins-*.db"))
	if len(dbs) == 0 {
		return nil, os.ErrNotExist
	}
	sort.Slice(dbs, func(i, j int) bool { return dbNumber(dbs[i]) > dbNumber(dbs[j]) })
	db, err := sqliteread.Open(dbs[0])
	if err != nil {
		return nil, err
	}
	rows, err := db.Records("plugins")
	if err != nil {
		return nil, err
	}
	files := map[int64]string{}
	if mods, err := db.Records("plugin_modules"); err == nil {
		for _, m := range mods {
			if id, ok := m["module_id"].(int64); ok {
				files[id] = str(m["path"])
			}
		}
	}
	out := []Plugin{}
	for _, r := range rows {
		id := str(r["dev_identifier"])
		p := Plugin{ID: id, Name: str(r["name"]), Vendor: str(r["vendor"]), Version: str(r["version"])}
		if mid, ok := r["module_id"].(int64); ok {
			p.File = files[mid]
		}
		switch {
		case strings.Contains(id, ":vst3:"):
			p.Format = "VST3"
		case strings.Contains(id, ":vst:"):
			p.Format = "VST"
		case strings.Contains(id, ":au:"):
			p.Format = "AU"
		}
		out = append(out, p)
	}
	return out, nil
}

func dbNumber(p string) int {
	s := strings.TrimSuffix(filepath.Base(p), ".db")
	n, _ := strconv.Atoi(s[strings.LastIndex(s, "-")+1:])
	return n
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// packs lists the packs installed: the folders in Live's Factory Packs
// folder, and the Core Library that comes with Live.
func packs(d Dirs, installs []Install) ([]string, bool) {
	cfg := newestPrefs(d, "Library.cfg")
	if cfg == "" {
		return nil, false
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		return nil, false
	}
	var lib struct {
		ContentLibrary struct {
			UserLibrary struct {
				LibraryProject []struct {
					ProjectPath struct {
						Value string `xml:"Value,attr"`
					}
				}
			}
			PreferredFactoryPacksInstallationPath struct {
				Value string `xml:"Value,attr"`
			}
		}
	}
	if xml.Unmarshal(data, &lib) != nil {
		return nil, false
	}
	dir := lib.ContentLibrary.PreferredFactoryPacksInstallationPath.Value
	if dir == "" {
		if ps := lib.ContentLibrary.UserLibrary.LibraryProject; len(ps) > 0 && ps[0].ProjectPath.Value != "" {
			dir = filepath.Join(filepath.FromSlash(ps[0].ProjectPath.Value), "Factory Packs")
		}
	}
	out := []string{}
	if dir != "" {
		entries, _ := os.ReadDir(dir)
		for _, en := range entries {
			if en.IsDir() {
				out = append(out, en.Name())
			}
		}
	}
	for _, in := range installs {
		if fi, err := os.Stat(filepath.Join(in.Path, "Resources", "Core Library")); err == nil && fi.IsDir() {
			out = append(out, "Core Library")
			break
		}
	}
	sort.Strings(out)
	return out, true
}

// newestPrefs finds name in the preferences of the newest Live that ran
// here (%APPDATA%\Ableton\Live 12.3.2\Preferences).
func newestPrefs(d Dirs, name string) string {
	dirs, _ := filepath.Glob(filepath.Join(d.AppData, "Ableton", "Live *"))
	best, bestVersion := "", ""
	for _, dir := range dirs {
		v := strings.TrimPrefix(filepath.Base(dir), "Live ")
		p := filepath.Join(dir, "Preferences", name)
		if _, err := os.Stat(p); err == nil && (best == "" || Compare(v, bestVersion) > 0) {
			best, bestVersion = p, v
		}
	}
	return best
}

// leadingInt reads the number a version part starts with ("1b5": 1).
func leadingInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Compare compares dotted version numbers ("12.1.5" < "12.3"); missing
// parts count as 0, "" is the oldest.
func Compare(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" || b == "" {
		if a == "" {
			return -1
		}
		return 1
	}
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = leadingInt(pa[i])
		}
		if i < len(pb) {
			y = leadingInt(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
