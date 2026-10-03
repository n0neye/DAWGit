package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/als"
)

// Inventory is what a project's folder holds, for a project check: read
// only (unlike a snapshot, external samples are not copied anywhere).
type Inventory struct {
	Files int   // tracked files: what a first version uploads
	Bytes int64 // their size
	// Ignored: files the rules leave out (Live's backups, caches; not the
	// folders left out whole), and their size.
	Ignored      int
	IgnoredBytes int64
	Sets         []SetInventory
	Samples      SampleInventory
}

// SetInventory is a Live Set of the project.
type SetInventory struct {
	Path    string
	Version string // the Live version that saved it ("12.1.5")
	Plugins []als.PluginRef
	Err     string // it couldn't be read
}

// SampleInventory sorts the samples the sets use.
type SampleInventory struct {
	InProject int         // in the project folder
	External  []FileEntry // elsewhere on this computer: kept with the versions
	Packs     []string    // from Live packs: not kept (each computer has its packs)
	PackRefs  int         // how many samples come from packs
	Missing   []string    // not found
}

// Inventory looks through the project folder.
func (r *Repo) Inventory() (*Inventory, error) {
	files, err := r.scan(true)
	if err != nil {
		return nil, err
	}
	inv := &Inventory{}
	seenIn, seenExt, seenPack, seenMissing := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range files {
		if f.ignored {
			inv.Ignored++
			inv.IgnoredBytes += f.size
			continue
		}
		inv.Files++
		inv.Bytes += f.size
		if !isSet(f.rel) {
			continue
		}
		si := SetInventory{Path: f.rel}
		s, err := als.Load(r.Abs(f.rel))
		if err != nil {
			si.Err = err.Error()
			inv.Sets = append(inv.Sets, si)
			continue
		}
		si.Version, si.Plugins = s.Version(), s.PluginRefs()
		inv.Sets = append(inv.Sets, si)
		for _, ref := range s.SampleRefs() {
			key := ref.Path + "|" + ref.RelativePath
			switch {
			case ref.Pack != "":
				inv.Samples.PackRefs++
				if !seenPack[ref.Pack] {
					seenPack[ref.Pack] = true
					inv.Samples.Packs = append(inv.Samples.Packs, ref.Pack)
				}
			case ref.RelativePathType == "3":
				if _, err := os.Stat(r.Abs(ref.RelativePath)); err != nil {
					if !seenMissing[ref.RelativePath] {
						seenMissing[ref.RelativePath] = true
						inv.Samples.Missing = append(inv.Samples.Missing, ref.RelativePath)
					}
				} else if !seenIn[key] {
					seenIn[key] = true
					inv.Samples.InProject++
				}
			case ref.Path != "":
				// Relinked to DAWGit's copy (downloaded): external, kept.
				kept := filepath.Join(r.Dir, externalDir)
				if rel, err := filepath.Rel(kept, filepath.FromSlash(ref.Path)); err == nil && !strings.HasPrefix(rel, "..") {
					if !seenExt[ref.Path] {
						seenExt[ref.Path] = true
						fi, _ := os.Stat(filepath.FromSlash(ref.Path))
						var n int64
						if fi != nil {
							n = fi.Size()
						}
						inv.Samples.External = append(inv.Samples.External, FileEntry{Path: ref.Path, Size: n})
					}
					continue
				}
				if r.inProject(ref.Path) {
					if !seenIn[key] {
						seenIn[key] = true
						inv.Samples.InProject++
					}
					continue
				}
				if seenExt[ref.Path] {
					continue
				}
				fi, err := os.Stat(filepath.FromSlash(ref.Path))
				if err != nil {
					if !seenMissing[ref.Path] {
						seenMissing[ref.Path] = true
						inv.Samples.Missing = append(inv.Samples.Missing, ref.Path)
					}
					continue
				}
				seenExt[ref.Path] = true
				inv.Samples.External = append(inv.Samples.External, FileEntry{Path: ref.Path, Size: fi.Size()})
			}
		}
	}
	sort.Strings(inv.Samples.Packs)
	sort.Strings(inv.Samples.Missing)
	sort.Slice(inv.Samples.External, func(i, j int) bool { return inv.Samples.External[i].Path < inv.Samples.External[j].Path })
	return inv, nil
}
