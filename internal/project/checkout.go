package project

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/xmltree"
)

// ErrDirty is returned by Checkout when working files differ from HEAD.
var ErrDirty = errors.New("working files have changes that are not in a snapshot")

// externalDir holds samples that live outside the project on the machine that
// made the snapshot, materialized here when missing locally.
const externalDir = "external"

// Checkout makes the working files match snapshot ref and relinks sample
// references for this machine. Unless force is set it refuses to overwrite
// changes that are not snapshotted.
func (r *Repo) Checkout(ref string, force bool) (*Manifest, []string, error) {
	id, err := r.Resolve(ref)
	if err != nil {
		return nil, nil, err
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, nil, err
	}
	if !force {
		changes, err := r.Status()
		if err != nil {
			return nil, nil, err
		}
		if len(changes) > 0 {
			return nil, nil, fmt.Errorf("%w (%d file(s); snapshot them or use --force)", ErrDirty, len(changes))
		}
	}

	ix := r.loadIndex()
	working, err := r.workingFiles(ix)
	if err != nil {
		return nil, nil, err
	}
	have := map[string]string{}
	for _, f := range working {
		have[f.Path] = f.Hash
	}
	want := m.FileMap()
	for _, f := range m.Files {
		if have[f.Path] == f.Hash {
			continue
		}
		if err := r.Store.Export(f.Hash, r.Abs(f.Path)); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if err := ix.record(r.Abs(f.Path), f.Path, f.Hash); err != nil {
			return nil, nil, err
		}
	}
	for p := range have {
		if _, ok := want[p]; !ok {
			if err := os.Remove(r.Abs(p)); err != nil {
				return nil, nil, err
			}
			delete(ix.entries, p)
			ix.dirty = true
		}
	}

	notes, err := r.relink(m, ix)
	if err != nil {
		return nil, nil, err
	}
	if err := r.setHead(m.ID); err != nil {
		return nil, nil, err
	}
	return m, notes, ix.save()
}

// relink rewrites sample paths in the checked-out sets so they resolve on this
// machine. A rewritten set keeps mapping to its snapshot object via the index.
func (r *Repo) relink(m *Manifest, ix *index) ([]string, error) {
	external := map[string]FileEntry{}
	for _, e := range m.External {
		external[e.Path] = e
	}
	var notes []string
	for _, f := range m.Files {
		if !isSet(f.Path) {
			continue
		}
		abs := r.Abs(f.Path)
		s, err := als.Load(abs)
		if err != nil {
			return nil, err
		}
		changed := false
		// Only sample references: FileRefs elsewhere (preset/device sources)
		// record paths on the content author's machine and are not loaded.
		for _, sr := range s.Root.Iter("SampleRef") {
			fr := sr.Child("FileRef")
			if fr == nil {
				continue
			}
			updated, note, err := r.relinkRef(fr, external)
			if err != nil {
				return nil, err
			}
			changed = changed || updated
			if note != "" {
				notes = append(notes, fmt.Sprintf("%s: %s", f.Path, note))
			}
		}
		if !changed {
			continue
		}
		if err := s.Save(abs); err != nil {
			return nil, err
		}
		if err := ix.record(abs, f.Path, f.Hash); err != nil {
			return nil, err
		}
	}
	return notes, nil
}

// relinkRef updates one sample FileRef. It reports whether it changed the
// set, and a note for the user when the change is worth mentioning.
func (r *Repo) relinkRef(fr *xmltree.Node, external map[string]FileEntry) (bool, string, error) {
	pathNode := fr.Child("Path")
	if pathNode == nil || fr.Val("LivePackName", "") != "" {
		return false, "", nil
	}
	orig := pathNode.Attr("Value")
	if rel := fr.Val("RelativePath", ""); fr.Val("RelativePathType", "") == "3" && rel != "" {
		// Project-relative: point the absolute path at this machine's project.
		// Routine, so no note.
		local := filepath.ToSlash(r.Abs(rel))
		if local == orig {
			return false, "", nil
		}
		pathNode.Set("Value", local)
		return true, "", nil
	}
	if orig == "" {
		return false, "", nil
	}
	if _, err := os.Stat(filepath.FromSlash(orig)); err == nil {
		return false, "", nil // exists here too
	}
	e, ok := external[orig]
	if !ok {
		e, ok = r.externalByCachePath(orig, external)
	}
	if !ok {
		return false, "missing sample " + orig, nil
	}
	if _, err := os.Stat(filepath.FromSlash(e.Path)); err == nil {
		// Relinked elsewhere, but the original file exists on this machine.
		pathNode.Set("Value", e.Path)
		return true, fmt.Sprintf("%s -> %s", orig, e.Path), nil
	}
	dst := r.externalCachePath(e)
	if _, err := os.Stat(dst); err != nil {
		if err := r.Store.Export(e.Hash, dst); err != nil {
			return false, "", err
		}
	}
	pathNode.Set("Value", filepath.ToSlash(dst))
	return true, fmt.Sprintf("%s -> %s", orig, filepath.ToSlash(dst)), nil
}

// externalCachePath is where an external sample is materialized:
// .dawgit/external/<hash>/<original file name>.
func (r *Repo) externalCachePath(e FileEntry) string {
	return filepath.Join(r.Dir, externalDir, e.Hash, path.Base(e.Path))
}

// externalByCachePath maps a path inside the external cache back to its
// manifest entry (a set relinked on this machine and snapshotted again).
func (r *Repo) externalByCachePath(p string, external map[string]FileEntry) (FileEntry, bool) {
	hash, ok := r.cacheHash(p)
	if !ok {
		return FileEntry{}, false
	}
	for _, e := range external {
		if e.Hash == hash {
			return e, true
		}
	}
	return FileEntry{}, false
}

// cacheHash returns the object hash if p points into an external cache — this
// machine's or a collaborator's (".../.dawgit/external/<hash>/<name>").
func (r *Repo) cacheHash(p string) (string, bool) {
	marker := "/" + metaDir + "/" + externalDir + "/"
	p = filepath.ToSlash(p)
	i := strings.Index(p, marker)
	if i < 0 {
		return "", false
	}
	hash, _, ok := strings.Cut(p[i+len(marker):], "/")
	return hash, ok && r.Store.Has(hash)
}
