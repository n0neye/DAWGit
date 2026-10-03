package health

import (
	"sort"
	"strings"
	"time"

	"dawgit/internal/liveenv"
	"dawgit/internal/project"
)

// Setup is what a member's computer has for the team's projects, shared
// with the team when they choose to: names and versions only (no paths, no
// files).
type Setup struct {
	Updated      string        `json:"updated"` // RFC 3339
	Live         []SetupLive   `json:"live"`
	Plugins      []SetupPlugin `json:"plugins"`
	PluginsKnown bool          `json:"pluginsKnown"`
	Packs        []string      `json:"packs"`
	PacksKnown   bool          `json:"packsKnown"`
}

// SetupLive is a Live installed.
type SetupLive struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
}

// SetupPlugin is a plugin Live found.
type SetupPlugin struct {
	Format  string `json:"format"`
	UID     string `json:"uid"`
	Name    string `json:"name"`
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
}

// SetupFrom is this computer's setup (env), as shared.
func SetupFrom(env *liveenv.Env, now time.Time) Setup {
	s := Setup{Updated: now.UTC().Format(time.RFC3339), Live: []SetupLive{}, Plugins: []SetupPlugin{},
		PluginsKnown: env.PluginsKnown, Packs: append([]string{}, env.Packs...), PacksKnown: env.PacksKnown}
	for _, in := range env.Installs {
		s.Live = append(s.Live, SetupLive{Version: in.Version, Edition: in.Edition})
	}
	for _, p := range env.Plugins {
		s.Plugins = append(s.Plugins, SetupPlugin{Format: p.Format, UID: p.UID(), Name: p.Name, Vendor: p.Vendor,
			Version: p.Version})
	}
	sort.Slice(s.Plugins, func(i, j int) bool {
		return s.Plugins[i].Format+s.Plugins[i].UID < s.Plugins[j].Format+s.Plugins[j].UID
	})
	return s
}

// Same: the setups list the same things (Updated aside).
func (s Setup) Same(o Setup) bool {
	s.Updated, o.Updated = "", ""
	return equalJSON(s, o)
}

// env is the setup as this package's view of a computer.
func (s Setup) env() *liveenv.Env {
	e := &liveenv.Env{PluginsKnown: s.PluginsKnown, Packs: s.Packs, PacksKnown: s.PacksKnown}
	for _, l := range s.Live {
		e.Installs = append(e.Installs, liveenv.Install{Version: l.Version, Edition: l.Edition})
	}
	for _, p := range s.Plugins {
		e.Plugins = append(e.Plugins, liveenv.Plugin{ID: "device:" + strings.ToLower(p.Format) + ":" + p.UID,
			Format: p.Format, Name: p.Name, Vendor: p.Vendor, Version: p.Version})
	}
	return e
}

// MemberCheck is a teammate's computer against the project, from the setup
// they shared.
type MemberCheck struct {
	Name    string `json:"name"`
	Updated string `json:"updated"`
	Live    string `json:"live"`  // their newest Live ("" none)
	Opens   string `json:"opens"` // "yes", "older", "none" (see LiveReport)
	// MissingPlugins: plugins the sets use that their Live didn't find;
	// OtherVersions: plugins they have in another version than this computer.
	MissingPlugins []string        `json:"missingPlugins"`
	OtherVersions  []PluginVersion `json:"otherVersions"`
	MissingPacks   []string        `json:"missingPacks"`
	PluginsKnown   bool            `json:"pluginsKnown"`
	PacksKnown     bool            `json:"packsKnown"`
}

// PluginVersion is a plugin in two versions: theirs and this computer's.
type PluginVersion struct {
	Name   string `json:"name"`
	Theirs string `json:"theirs"`
	Here   string `json:"here"`
}

// CheckTeam adds the teammates' setups (by name) to a report of inv.
func CheckTeam(rep *Report, inv *project.Inventory, setups map[string]Setup) {
	rep.Team = []MemberCheck{}
	if rep.Live == nil {
		return
	}
	names := make([]string, 0, len(setups))
	for n := range setups {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		s := setups[name]
		theirs := Check(inv, s.env()).Live
		mc := MemberCheck{Name: name, Updated: s.Updated, Opens: theirs.Opens, MissingPlugins: []string{},
			OtherVersions: []PluginVersion{}, MissingPacks: []string{}, PluginsKnown: s.PluginsKnown, PacksKnown: s.PacksKnown}
		if n := s.env().Newest(); n != nil {
			mc.Live = n.Version
		}
		for i, p := range theirs.Plugins {
			switch {
			case p.Here == "no":
				mc.MissingPlugins = append(mc.MissingPlugins, p.Name)
			case p.Here == "yes" && rep.Live.Plugins[i].Here == "yes" && p.Version != rep.Live.Plugins[i].Version:
				mc.OtherVersions = append(mc.OtherVersions, PluginVersion{Name: p.Name, Theirs: p.Version,
					Here: rep.Live.Plugins[i].Version})
			}
		}
		for _, p := range theirs.Samples.Packs {
			if p.Here == "no" {
				mc.MissingPacks = append(mc.MissingPacks, p.Name)
			}
		}
		rep.Team = append(rep.Team, mc)
	}
}
