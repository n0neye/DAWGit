package project

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/store"
	"dawgit/internal/xmltree"
)

// Samples the project's sets use that DAWGit can put in the project folder:
// a missing one DAWGit has a copy of (an external drive gone, a folder
// deleted: committed versions kept its content), or one only in .dawgit (a
// teammate's sample, downloaded there; gone if .dawgit is). Brought in, they
// are ordinary project files: in Samples/Imported (where Live's Collect All
// and Save puts them), or back at their place in the project.

// importedDir is where samples from outside the project go.
const importedDir = "Samples/Imported"

// SampleSpot is a sample the sets refer to that is missing or only in .dawgit.
type SampleSpot struct {
	Path    string   // as the sets refer to it: absolute, or relative to the project
	Name    string   // its file name
	Hash    string   // its content, "" when DAWGit has no copy
	Size    int64    // bytes (0 when unknown)
	Missing bool     // not where the sets refer to it
	Kept    bool     // in .dawgit
	Sets    []string // the sets that use it
}

// Restorable: DAWGit can bring it back.
func (s SampleSpot) Restorable() bool { return s.Missing && s.Hash != "" }

// sampleKey is how a set refers to a sample: its project-relative path, or
// its absolute path. inside: the sample is (meant to be) in the project.
func (r *Repo) sampleKey(ref als.SampleRef) (key string, inside bool) {
	if ref.RelativePathType == "3" && ref.RelativePath != "" {
		return ref.RelativePath, true
	}
	if ref.Path == "" {
		return "", false
	}
	if r.inProject(ref.Path) && !strings.Contains(filepath.ToSlash(ref.Path), "/"+metaDir+"/") {
		rel, _ := filepath.Rel(r.Root, filepath.FromSlash(ref.Path))
		return filepath.ToSlash(rel), true
	}
	return ref.Path, false
}

// SampleSpots lists the samples the working sets use that are missing or
// only in .dawgit.
func (r *Repo) SampleSpots() ([]SampleSpot, error) {
	files, err := r.scan(false)
	if err != nil {
		return nil, err
	}
	known, err := r.knownSamples()
	if err != nil {
		return nil, err
	}
	spots := map[string]*SampleSpot{}
	var order []string
	for _, f := range files {
		if !isSet(f.rel) {
			continue
		}
		s, err := als.Load(r.Abs(f.rel))
		if err != nil {
			continue // a set that can't be read: nothing to bring in for it
		}
		for _, ref := range s.SampleRefs() {
			if ref.Pack != "" {
				continue
			}
			key, inside := r.sampleKey(ref)
			if key == "" {
				continue
			}
			if sp := spots[key]; sp != nil {
				if !slices.Contains(sp.Sets, f.rel) {
					sp.Sets = append(sp.Sets, f.rel)
				}
				continue
			}
			abs := filepath.FromSlash(key)
			if inside {
				abs = r.Abs(key)
			}
			sp := SampleSpot{Path: key, Name: path.Base(filepath.ToSlash(key)), Sets: []string{f.rel}}
			if hash, ok := r.cacheHash(abs); ok { // in .dawgit
				sp.Kept, sp.Hash = true, hash
				if fi, err := os.Stat(abs); err != nil {
					sp.Missing = true
				} else {
					sp.Size = fi.Size()
				}
			} else if _, err := os.Stat(abs); err != nil {
				sp.Missing = true
				if e, ok := known[key]; ok {
					sp.Hash, sp.Size = e.Hash, e.Size
				}
			} else {
				continue // where it should be
			}
			spots[key] = &sp
			order = append(order, key)
		}
	}
	out := make([]SampleSpot, 0, len(order))
	for _, k := range order {
		out = append(out, *spots[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// knownSamples maps the samples versions kept to their contents: external
// samples by their absolute path (as committed), project files by their
// relative path (in the version the project is on, and the latest).
func (r *Repo) knownSamples() (map[string]FileEntry, error) {
	out := map[string]FileEntry{}
	ids, err := r.storedVersions()
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		m, err := r.readRecord(id)
		if err != nil {
			continue
		}
		for _, e := range m.External {
			out[e.Path] = e
		}
	}
	for _, id := range []string{r.Latest(), r.Head()} {
		if id == "" {
			continue
		}
		m, err := r.Load(id)
		if err != nil {
			return nil, err
		}
		for _, f := range m.Files {
			out[f.Path] = f
		}
	}
	return out, nil
}

// BringSamplesIn puts samples into the project folder and points the sets
// at them: the missing ones DAWGit has a copy of (missing), and those only
// in .dawgit (kept). It returns how many. The sets and the new files show
// as changes, to commit; the originals elsewhere are not touched.
func (r *Repo) BringSamplesIn(missing, kept bool) (int, error) {
	spots, err := r.SampleSpots()
	if err != nil {
		return 0, err
	}
	var todo []SampleSpot
	var hashes []string
	for _, sp := range spots {
		if sp.Hash != "" && ((missing && sp.Missing) || (kept && sp.Kept)) {
			todo = append(todo, sp)
			hashes = append(hashes, sp.Hash)
		}
	}
	if len(todo) == 0 {
		return 0, nil
	}
	if err := r.ensureHashes(hashes); err != nil {
		return 0, err
	}
	// Where each goes: back to its place in the project, or Samples/Imported
	// (another name if one with other content is there).
	dest := map[string]string{} // sample key -> project-relative path
	taken := map[string]string{}
	for _, sp := range todo {
		rel := sp.Path
		if filepath.IsAbs(filepath.FromSlash(sp.Path)) || sp.Kept {
			rel = r.importedPath(sp, taken)
		}
		abs := r.Abs(rel)
		if _, err := os.Stat(abs); err != nil {
			if err := r.exportObject(sp.Hash, abs); err != nil {
				return 0, fmt.Errorf("%s: %w", sp.Name, err)
			}
		}
		dest[sp.Path] = rel
		taken[rel] = sp.Hash
	}
	// The sets point at them.
	files, err := r.scan(false)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		if !isSet(f.rel) {
			continue
		}
		abs := r.Abs(f.rel)
		s, err := als.Load(abs)
		if err != nil {
			continue
		}
		changed := false
		for _, srn := range s.Root.Iter("SampleRef") {
			fr := srn.Child("FileRef")
			if fr == nil {
				continue
			}
			ref := als.SampleRef{Path: fr.Val("Path", ""), RelativePath: fr.Val("RelativePath", ""),
				RelativePathType: fr.Val("RelativePathType", ""), Pack: fr.Val("LivePackName", "")}
			if ref.Pack != "" {
				continue
			}
			key, _ := r.sampleKey(ref)
			rel, ok := dest[key]
			if !ok {
				continue
			}
			if r.pointAt(fr, r.Abs(rel)) {
				changed = true
			}
			if setValue(fr.Child("RelativePathType"), "3") { // relative to the project
				changed = true
			}
		}
		if changed {
			if err := s.Save(abs); err != nil {
				return 0, err
			}
		}
	}
	return len(todo), nil
}

// importedPath is where a sample from outside goes: Samples/Imported/<name>,
// or <name> (2) and so on when another file has that name.
func (r *Repo) importedPath(sp SampleSpot, taken map[string]string) string {
	ext := path.Ext(sp.Name)
	base := strings.TrimSuffix(sp.Name, ext)
	for i := 1; ; i++ {
		name := sp.Name
		if i > 1 {
			name = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		rel := importedDir + "/" + name
		if h, ok := taken[rel]; ok {
			if h == sp.Hash {
				return rel
			}
			continue
		}
		if _, err := os.Stat(r.Abs(rel)); err == nil {
			if h, _, err := store.HashFile(r.Abs(rel)); err == nil && h == sp.Hash {
				return rel // already there
			}
			continue
		}
		return rel
	}
}

func setValue(n *xmltree.Node, v string) bool {
	if n == nil || n.Attr("Value") == v {
		return false
	}
	n.Set("Value", v)
	return true
}
