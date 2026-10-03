// Package health checks a project: what it holds and needs (for Live sets:
// the Live version, samples, plugins, packs) against what this computer
// has. It reports, it never changes anything: the app shows it when a
// project is added (before its first version) and after one is downloaded.
package health

import (
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/liveenv"
	"dawgit/internal/project"
)

// Report is a project's check.
type Report struct {
	Files        int         `json:"files"` // tracked: what a first version uploads
	Bytes        int64       `json:"bytes"`
	Ignored      int         `json:"ignored"` // left out by the rules
	IgnoredBytes int64       `json:"ignoredBytes"`
	Live         *LiveReport `json:"live"` // nil: no Live Sets
}

// LiveReport is about the project's Live Sets.
type LiveReport struct {
	Sets []SetLine `json:"sets"`
	// Needs: the newest Live version that saved one of the sets.
	Needs     string            `json:"needs"`
	Installed []liveenv.Install `json:"installed"`
	// Opens: "yes" (this computer's newest Live is as new as Needs),
	// "older" (it is older: Live can't open sets saved by a newer Live),
	// "none" (no Live found here).
	Opens   string       `json:"opens"`
	Samples SampleReport `json:"samples"`
	Plugins []PluginLine `json:"plugins"`
	// PluginsKnown: Live's plugin list could be read (else Here is
	// "unknown" for every plugin).
	PluginsKnown bool `json:"pluginsKnown"`
}

// SetLine is one set.
type SetLine struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Err     string `json:"err"`
}

// SampleReport counts the samples the sets use.
type SampleReport struct {
	InProject     int        `json:"inProject"`
	External      int        `json:"external"` // outside the project: kept with the versions
	ExternalBytes int64      `json:"externalBytes"`
	PackRefs      int        `json:"packRefs"` // from packs: each computer needs the pack
	Packs         []PackLine `json:"packs"`
	Missing       []string   `json:"missing"` // not found: saved as missing
}

// PackLine is a pack the sets use; Here: "yes", "no" or "unknown".
type PackLine struct {
	Name string `json:"name"`
	Here string `json:"here"`
}

// PluginLine is a third-party plugin the sets use.
type PluginLine struct {
	Name   string   `json:"name"`
	Format string   `json:"format"`
	Sets   []string `json:"sets"`
	// Here: "yes" (Live found it on this computer), "no", or "unknown".
	Here    string `json:"here"`
	Vendor  string `json:"vendor"`  // from Live's list, when here
	Version string `json:"version"` // the version here
}

// Check looks at a project's inventory with this computer's Live (env).
func Check(inv *project.Inventory, env *liveenv.Env) *Report {
	rep := &Report{Files: inv.Files, Bytes: inv.Bytes, Ignored: inv.Ignored, IgnoredBytes: inv.IgnoredBytes}
	if len(inv.Sets) == 0 {
		return rep
	}
	lr := &LiveReport{Sets: []SetLine{}, Installed: env.Installs, Plugins: []PluginLine{}, PluginsKnown: env.PluginsKnown}
	plugins := map[string]*PluginLine{}
	var order []string
	for _, s := range inv.Sets {
		lr.Sets = append(lr.Sets, SetLine{Path: s.Path, Version: s.Version, Err: s.Err})
		if liveenv.Compare(s.Version, lr.Needs) > 0 {
			lr.Needs = s.Version
		}
		for _, p := range s.Plugins {
			key := p.Format + "|" + p.UID + "|" + p.Name
			pl := plugins[key]
			if pl == nil {
				pl = &PluginLine{Name: p.Name, Format: p.Format, Sets: []string{}, Here: "unknown"}
				if env.PluginsKnown {
					pl.Here = "no"
					if got := find(env.Plugins, p); got != nil {
						pl.Here, pl.Vendor, pl.Version = "yes", got.Vendor, got.Version
					}
				}
				plugins[key] = pl
				order = append(order, key)
			}
			pl.Sets = append(pl.Sets, s.Path)
		}
	}
	for _, k := range order {
		lr.Plugins = append(lr.Plugins, *plugins[k])
	}
	sort.SliceStable(lr.Plugins, func(i, j int) bool {
		return strings.ToLower(lr.Plugins[i].Name) < strings.ToLower(lr.Plugins[j].Name)
	})
	switch newest := env.Newest(); {
	case newest == nil:
		lr.Opens = "none"
	case liveenv.Compare(newest.Version, lr.Needs) >= 0:
		lr.Opens = "yes"
	default:
		lr.Opens = "older"
	}

	s := inv.Samples
	lr.Samples = SampleReport{InProject: s.InProject, External: len(s.External), PackRefs: s.PackRefs,
		Packs: []PackLine{}, Missing: nonNil(s.Missing)}
	for _, e := range s.External {
		lr.Samples.ExternalBytes += e.Size
	}
	have := map[string]bool{}
	for _, p := range env.Packs {
		have[strings.ToLower(p)] = true
	}
	for _, p := range s.Packs {
		here := "unknown"
		if env.PacksKnown {
			here = "no"
			if have[strings.ToLower(p)] {
				here = "yes"
			}
		}
		lr.Samples.Packs = append(lr.Samples.Packs, PackLine{Name: p, Here: here})
	}
	rep.Live = lr
	return rep
}

// find is the plugin here that a set's plugin is: a VST3 by its class id,
// a VST2 by its id, file name or name.
func find(here []liveenv.Plugin, p als.PluginRef) *liveenv.Plugin {
	for i := range here {
		h := &here[i]
		if h.Format != p.Format {
			continue
		}
		switch p.Format {
		case "VST3":
			if p.UID != "" && strings.EqualFold(h.UID(), p.UID) {
				return h
			}
		case "VST":
			base := strings.TrimSuffix(filepath.Base(h.File), filepath.Ext(h.File))
			if (p.UID != "" && h.UID() == p.UID) ||
				(p.File != "" && strings.EqualFold(filepath.Base(h.File), p.File)) ||
				strings.EqualFold(h.Name, p.Name) || strings.EqualFold(base, p.Name) {
				return h
			}
		default:
			if strings.EqualFold(h.Name, p.Name) {
				return h
			}
		}
	}
	return nil
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
