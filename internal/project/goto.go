package project

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/als"
)

// ErrOlderVersion is returned by actions that would add to the history while
// the project is on an older version (see GoTo).
var ErrOlderVersion = errors.New("the project is on an older version: go back to the latest version, or start a branch from here")

// Latest is the newest version of the current branch on this computer: the
// version the project is on, unless GoTo moved it to an older one.
func (r *Repo) Latest() string {
	if r.Config.Tip != "" {
		return r.Config.Tip
	}
	return r.Head()
}

// InBranch returns the versions the current branch already contains (the
// latest version and everything before it).
func (r *Repo) InBranch() (map[string]bool, error) {
	return r.ancestors(r.Latest())
}

// OnOlderVersion reports whether GoTo moved the project to an older version.
func (r *Repo) OnOlderVersion() bool { return r.Config.Tip != "" }

func (r *Repo) guardLatest() error {
	if r.OnOlderVersion() {
		return ErrOlderVersion
	}
	return nil
}

// ensureObjects makes the files of m available on this computer, downloading
// them from the team when needed.
func (r *Repo) ensureObjects(m *Manifest) error {
	r.knowSizes(m)
	return r.ensureHashes(m.Objects())
}

// GoTo puts the project folder in the state of a version ("latest" for the
// newest). Newer versions are kept: Latest remembers where the branch is.
// Uncommitted changes make it fail with ErrDirty unless discard is set.
func (r *Repo) GoTo(ref string, discard bool) (*Manifest, []string, error) {
	latest := r.Latest()
	if ref == "latest" {
		ref = latest
	}
	id, err := r.Resolve(ref)
	if err != nil {
		return nil, nil, err
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, nil, err
	}
	if err := r.ensureObjects(m); err != nil {
		return nil, nil, err
	}
	m, notes, err := r.Checkout(id, discard)
	if err != nil {
		return nil, nil, err
	}
	r.Config.Tip = ""
	if id != latest {
		r.Config.Tip = latest
	}
	if err := r.SaveConfig(); err != nil {
		return nil, nil, err
	}
	if r.OnOlderVersion() && r.Config.Remote != nil {
		r.AdoptBranchAtHead()
	}
	return m, notes, nil
}

// AdoptBranchAtHead: a project on an "older" version that is the latest
// version of a branch is simply on that branch, so commits go there. It stays
// put when the current branch has versions not shared yet (they would be
// hard to find again). It reports whether it switched.
func (r *Repo) AdoptBranchAtHead() bool {
	if !r.OnOlderVersion() {
		return false
	}
	c, err := r.Client()
	if err != nil {
		return false
	}
	heads, err := c.Branches(r.Config.ProjectID)
	if err != nil {
		return false
	}
	var names []string
	for name, h := range heads {
		if h == r.Head() {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return false
	}
	sort.Strings(names)
	if cur := heads[r.BranchName()]; r.Config.Tip != cur {
		if shared, err := r.isAncestor(r.Config.Tip, cur); err != nil || !shared {
			return false
		}
	}
	r.Config.Branch, r.Config.Tip = names[0], ""
	for _, n := range names {
		if n == "main" {
			r.Config.Branch = n
		}
	}
	return r.SaveConfig() == nil
}

// KeepThisVersion continues from the older version the project is on: its
// content (with any changes made since) becomes a new version on top of the
// latest one. Nothing in the history is lost.
func (r *Repo) KeepThisVersion(message string) (*Manifest, error) {
	if !r.OnOlderVersion() {
		return nil, errors.New("the project is already on its latest version")
	}
	if err := r.setHead(r.Config.Tip); err != nil {
		return nil, err
	}
	r.Config.Tip = ""
	if err := r.SaveConfig(); err != nil {
		return nil, err
	}
	return r.Snapshot(message)
}

// CommitOnOlderVersion commits the changes made on an older version as a
// version after it (not after the latest), and leaves the older-version
// state: the branch now has two lines to combine, which Save (or Update) then
// merges track by track, as when teammates committed in the meantime.
func (r *Repo) CommitOnOlderVersion(message string) (*Manifest, error) {
	if !r.OnOlderVersion() {
		return nil, errors.New("the project is already on its latest version")
	}
	tip := r.Config.Tip
	r.Config.Tip = ""
	m, err := r.Snapshot(message)
	if err != nil && !errors.Is(err, ErrNothingToSnapshot) {
		r.Config.Tip = tip
		return nil, err
	}
	return m, r.SaveConfig()
}

// ExportName is the default folder name for an exported version, e.g.
// "Night Drive (2026-09-28, 3f9c2a1b) Project".
func (r *Repo) ExportName(m *Manifest) string {
	name := r.Config.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(r.Root), " Project")
	}
	date := m.Time
	if len(date) >= 10 {
		date = date[:10]
	}
	return fmt.Sprintf("%s (%s, %s) Project", name, date, short(m.ID)[:8])
}

// Export writes a version as a separate Ableton project in dir, which must
// not exist yet or be empty. Samples from outside the project are copied into
// Samples/Imported and the sets point at them, so the copy opens on its own.
func (r *Repo) Export(ref, dir string) (*Manifest, error) {
	id, err := r.Resolve(ref)
	if err != nil {
		return nil, err
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s is not empty", dir)
	}
	if err := r.ensureObjects(m); err != nil {
		return nil, err
	}
	for i, f := range m.Files {
		r.report(StageExporting, i, len(m.Files))
		if err := r.exportObject(f.Hash, filepath.Join(dir, filepath.FromSlash(f.Path))); err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	x := &exporter{r: r, copy: &Repo{Root: dir, Dir: r.Dir, Store: r.Store}, external: map[string]FileEntry{},
		imported: map[string]string{}}
	for _, e := range m.External {
		x.external[e.Path] = e
	}
	for _, f := range m.Files {
		if isSet(f.Path) {
			if err := x.relinkSet(filepath.Join(dir, filepath.FromSlash(f.Path))); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Path, err)
			}
		}
	}
	return m, nil
}

type exporter struct {
	r, copy  *Repo
	external map[string]FileEntry
	imported map[string]string // object hash -> file in the copy
}

func (x *exporter) relinkSet(abs string) error {
	s, err := als.Load(abs)
	if err != nil {
		return err
	}
	changed := false
	for _, sr := range s.Root.Iter("SampleRef") {
		fr := sr.Child("FileRef")
		if fr == nil || fr.Child("Path") == nil || fr.Val("LivePackName", "") != "" {
			continue
		}
		orig := fr.Child("Path").Attr("Value")
		rel := fr.Val("RelativePath", "")
		e, ok := x.r.externalByCachePath(orig, x.external)
		if !ok {
			e, ok = x.r.externalByCachePath("/"+rel, x.external)
		}
		if !ok && fr.Val("RelativePathType", "") == "3" && rel != "" {
			changed = x.copy.pointAt(fr, x.copy.Abs(rel)) || changed
			continue
		}
		if !ok {
			e, ok = x.external[orig]
		}
		if !ok {
			continue // a sample this version did not store (still where it was)
		}
		target, err := x.importSample(e)
		if err != nil {
			return err
		}
		changed = x.copy.pointAt(fr, target) || changed
		if n := fr.Child("RelativePathType"); n != nil && n.Attr("Value") != "3" {
			n.Set("Value", "3") // relative to the project, like Live's "Collect All and Save"
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.Save(abs)
}

// importSample copies an external sample into Samples/Imported of the copy.
func (x *exporter) importSample(e FileEntry) (string, error) {
	if p, ok := x.imported[e.Hash]; ok {
		return p, nil
	}
	dir := filepath.Join(x.copy.Root, "Samples", "Imported")
	name := path.Base(e.Path)
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target); err == nil { // another sample with that name
		ext := path.Ext(name)
		target = filepath.Join(dir, strings.TrimSuffix(name, ext)+" "+e.Hash[:8]+ext)
	}
	if err := x.r.exportObject(e.Hash, target); err != nil {
		return "", err
	}
	x.imported[e.Hash] = target
	return target, nil
}
