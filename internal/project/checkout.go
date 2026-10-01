package project

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"dawgit/internal/als"
	"dawgit/internal/store"
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
	// Files the version's rules leave out stay on disk: when a teammate
	// starts ignoring a folder, it is not deleted from everyone's computer.
	target, err := r.profileOf(m)
	if err != nil {
		return nil, nil, err
	}
	want := m.FileMap()
	var need []string
	for _, f := range m.Files {
		if have[f.Path] != f.Hash {
			need = append(need, f.Hash)
		}
	}
	r.knowSizes(m)
	if err := r.ensureHashes(need); err != nil {
		return nil, nil, err
	}
	// Files change from here on: a switch that stops halfway (a crash, the
	// power) is known until it's done, and can be finished (UnfinishedSwitch).
	if err := os.WriteFile(filepath.Join(r.Dir, switchingFile), []byte(id+"\n"), 0o644); err != nil {
		return nil, nil, err
	}
	// Removed files go first: on Windows a file renamed only in case
	// ("Kick.wav" to "kick.wav") is the same file, and removing the old name
	// after writing the new one would remove it.
	for p := range have {
		if _, ok := want[p]; !ok && !target.Ignored(p, false) {
			if err := store.Remove(r.Abs(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, nil, err
			}
			delete(ix.entries, p)
			ix.dirty = true
		}
	}
	for _, f := range m.Files {
		if have[f.Path] == f.Hash {
			continue
		}
		if err := r.exportObject(f.Hash, r.Abs(f.Path)); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if err := ix.record(r.Abs(f.Path), f.Path, f.Hash, f.Size); err != nil {
			return nil, nil, err
		}
	}
	r.forgetProfile() // the version may have brought another .dawgit.yaml

	notes, err := r.relink(m, ix)
	if err != nil {
		return nil, nil, err
	}
	if err := r.setHead(m.ID); err != nil {
		return nil, nil, err
	}
	if err := ix.save(); err != nil {
		return nil, nil, err
	}
	os.Remove(filepath.Join(r.Dir, switchingFile))
	return m, notes, nil
}

// switchingFile names the version a checkout is putting in place.
const switchingFile = "switching"

// UnfinishedSwitch returns the version a checkout was putting in place when
// it stopped ("" when none): the project's files are partly that version.
// FinishSwitch completes it.
func (r *Repo) UnfinishedSwitch() string {
	data, err := os.ReadFile(filepath.Join(r.Dir, switchingFile))
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(data))
	if id == "" || id == r.Head() || !r.HasSnapshot(id) {
		os.Remove(filepath.Join(r.Dir, switchingFile))
		return ""
	}
	return id
}

// FinishSwitch completes a checkout that stopped halfway. Changes made since
// to the files it was replacing are lost, as they would have been.
func (r *Repo) FinishSwitch() ([]string, error) {
	id := r.UnfinishedSwitch()
	if id == "" {
		return nil, nil
	}
	_, notes, err := r.Checkout(id, true)
	return notes, err
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
		if err := ix.record(abs, f.Path, f.Hash, f.Size); err != nil {
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
	rel := fr.Val("RelativePath", "")
	// A path into someone's external cache (possibly saved by Live as
	// project-relative) is resolved like the external sample it stands for.
	e, cached := r.externalByCachePath(orig, external)
	if !cached {
		e, cached = r.externalByCachePath("/"+rel, external)
	}
	if !cached && fr.Val("RelativePathType", "") == "3" && rel != "" {
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
	if _, err := os.Stat(filepath.FromSlash(orig)); err == nil && !cached {
		return false, "", nil // exists here too
	}
	// Cached paths are always re-resolved: pointing into another project's
	// .dawgit would break when that project moves.
	ok := cached
	if !ok {
		e, ok = external[orig]
	}
	if !ok {
		return false, "missing sample " + orig, nil
	}
	target := filepath.FromSlash(e.Path)
	if _, err := os.Stat(target); err != nil {
		target = r.externalCachePath(e)
		if _, err := os.Stat(target); err != nil {
			if err := r.ensureHashes([]string{e.Hash}); err != nil {
				return false, "", err
			}
			if err := r.exportObject(e.Hash, target); err != nil {
				return false, "", err
			}
		}
	} // else: relinked elsewhere, but the original file exists on this machine
	if !r.pointAt(fr, target) {
		return false, "", nil
	}
	return true, fmt.Sprintf("%s -> %s", orig, filepath.ToSlash(target)), nil
}

// pointAt sets a FileRef's absolute path and its project-relative path (Live
// stores both for samples outside the project, e.g. "../Samples/x.wav").
// It reports whether anything changed.
func (r *Repo) pointAt(fr *xmltree.Node, abs string) bool {
	changed := false
	set := func(n *xmltree.Node, v string) {
		if n != nil && n.Attr("Value") != v {
			n.Set("Value", v)
			changed = true
		}
	}
	set(fr.Child("Path"), filepath.ToSlash(abs))
	if rel, err := filepath.Rel(r.Root, abs); err == nil {
		set(fr.Child("RelativePath"), filepath.ToSlash(rel))
	}
	return changed
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
	return hash, ok && (r.Store.Has(hash) || r.remoteOnly()[hash] || r.sourcesByHash()[hash] != "")
}
