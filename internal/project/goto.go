package project

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
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
	var need []string
	for _, h := range m.Objects() {
		if !r.Store.Has(h) {
			need = append(need, h)
		}
	}
	if len(need) == 0 {
		return nil
	}
	c, err := r.Client()
	if err != nil {
		return fmt.Errorf("some files of this version are not on this computer: %w", err)
	}
	return r.fetchObjects(c, need)
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
	return m, notes, r.SaveConfig()
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
		if err := r.Store.Export(f.Hash, filepath.Join(dir, filepath.FromSlash(f.Path))); err != nil {
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
	if err := x.r.Store.Export(e.Hash, target); err != nil {
		return "", err
	}
	x.imported[e.Hash] = target
	return target, nil
}
