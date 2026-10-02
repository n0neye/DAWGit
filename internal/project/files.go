package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"

	"dawgit/internal/diff"
	"dawgit/internal/store"
)

// ProjectFile is a file in the project folder as the app lists it.
type ProjectFile struct {
	Path   string // slash separated, relative to the project folder
	Status string // "added" | "modified" | "deleted" | "renamed" | "unchanged" | "ignored"
	Size   int64  // on disk (0 for deleted files)
	From   string // renamed: where it was
	Edited bool   // renamed: its content changed too
}

// Files lists the changed files; with all, also unchanged ones. Files the
// rules leave out (Live's backups, caches) aren't listed, except those that
// were tracked until now.
func (r *Repo) Files(all bool) ([]ProjectFile, error) {
	// (scan can list what the rules leave out too: an option one day)
	entries, err := r.scan(false)
	if err != nil {
		return nil, err
	}
	var tracked []scanned
	for _, e := range entries {
		if !e.ignored {
			tracked = append(tracked, e)
		}
	}
	ix := r.loadIndex()
	files, err := r.hashScanned(ix, tracked)
	if err != nil {
		return nil, err
	}
	defer ix.save()
	changes, err := r.statusOf(files)
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	moved := map[string]Change{}
	var out []ProjectFile
	for _, c := range changes {
		status[c.Path] = c.Status
		if c.Status == "renamed" {
			moved[c.Path] = c
		}
		if c.Status == "deleted" {
			out = append(out, ProjectFile{Path: c.Path, Status: "deleted"})
		}
	}
	if !all {
		// Only the changes; "untracked" ones aren't among the scanned files.
		size := map[string]int64{}
		for _, e := range entries {
			size[e.rel] = e.size
		}
		for _, c := range changes {
			if c.Status == "deleted" {
				continue
			}
			n, ok := size[c.Path]
			if !ok {
				if fi, err := os.Stat(r.Abs(c.Path)); err == nil {
					n = fi.Size()
				}
			}
			out = append(out, ProjectFile{Path: c.Path, Status: c.Status, Size: n, From: c.From, Edited: c.Edited})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
		return out, nil
	}
	listed := map[string]bool{}
	for _, e := range entries {
		listed[e.rel] = true
		st, changed := status[e.rel]
		switch {
		case changed:
		case e.ignored:
			st = "ignored"
		default:
			st = "unchanged"
		}
		out = append(out, ProjectFile{Path: e.rel, Status: st, Size: e.size, From: moved[e.rel].From, Edited: moved[e.rel].Edited})
	}
	// Tracked until now, in a folder the rules now leave out whole: listed
	// still, the next version won't have them.
	for _, c := range changes {
		if c.Status == "untracked" && !listed[c.Path] {
			pf := ProjectFile{Path: c.Path, Status: c.Status}
			if fi, err := os.Stat(r.Abs(c.Path)); err == nil {
				pf.Size = fi.Size()
			}
			out = append(out, pf)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func fileHash(m *Manifest, path string) string {
	for _, f := range m.Files {
		if f.Path == path {
			return f.Hash
		}
	}
	return ""
}

// FileVersion is a version that changed a file.
type FileVersion struct {
	Version *Manifest
	Status  string // "added" | "modified" | "deleted" | "renamed" (moved here from From)
	Hash    string // the file's content in that version ("" when deleted)
	Path    string // where the file is in that version
	From    string // renamed: where it was before
}

// FileHistory lists the versions of the current branch (newest first) that
// added, changed, moved or deleted path, following it back through moves:
// older versions list it where it was then.
func (r *Repo) FileHistory(path string) ([]FileVersion, error) {
	log, err := r.Log()
	if err != nil {
		return nil, err
	}
	// The file's content in each version, each version and path read once.
	hashes := map[string]string{}
	hashIn := func(id, p string) string {
		key := id + "|" + p
		h, ok := hashes[key]
		if !ok {
			if m, err := r.Load(id); err == nil {
				h = fileHash(m, p)
			}
			hashes[key] = h
		}
		return h
	}
	var out []FileVersion
	for _, m := range log {
		cur := hashIn(m.ID, path)
		prev := ""
		if len(m.Parents) > 0 {
			prev = hashIn(m.Parents[0], path)
		}
		switch {
		case cur == prev:
			continue
		case prev == "" && len(m.Parents) > 0:
			// New here: moved from somewhere else?
			if from := r.movedFrom(m, path); from != "" {
				out = append(out, FileVersion{Version: m, Status: "renamed", Hash: cur, Path: path, From: from})
				path = from
				continue
			}
			out = append(out, FileVersion{Version: m, Status: "added", Hash: cur, Path: path})
		case prev == "":
			out = append(out, FileVersion{Version: m, Status: "added", Hash: cur, Path: path})
		case cur == "":
			out = append(out, FileVersion{Version: m, Status: "deleted", Path: path})
		default:
			out = append(out, FileVersion{Version: m, Status: "modified", Hash: cur, Path: path})
		}
	}
	return out, nil
}

// movedFrom is where version m moved path from (compared with its first
// parent), or "".
func (r *Repo) movedFrom(m *Manifest, path string) string {
	parent, err := r.Load(m.Parents[0])
	if err != nil {
		return ""
	}
	full, err := r.Load(m.ID)
	if err != nil {
		return ""
	}
	for _, mv := range r.renamesBetween(parent, full) {
		if mv.to == path {
			return mv.from
		}
	}
	return ""
}

// FileDiff compares a set between two versions ("" for the version before
// the first: nothing).
func (r *Repo) FileDiff(path, from, to string) (*diff.SetDiff, error) {
	hash := func(ref string) (string, error) {
		if ref == "" {
			return "", nil
		}
		id, err := r.Resolve(ref)
		if err != nil {
			return "", err
		}
		m, err := r.Load(id)
		if err != nil {
			return "", err
		}
		f, ok := m.FileMap()[path]
		if !ok {
			return "", nil
		}
		if err := r.fetchFile(f); err != nil {
			return "", err
		}
		return f.Hash, nil
	}
	a, err := hash(from)
	if err != nil {
		return nil, err
	}
	b, err := hash(to)
	if err != nil {
		return nil, err
	}
	if a == "" || b == "" || !isSet(path) {
		return nil, nil
	}
	return r.storedSetDiff(a, b), nil
}

// fetchFile downloads one file of a version when it is not here yet.
func (r *Repo) fetchFile(f FileEntry) error {
	r.knowSizes(&Manifest{Files: []FileEntry{f}})
	return r.ensureHashes([]string{f.Hash})
}

// OpenFile opens path as it is in a version ("" for the project folder now).
func (r *Repo) OpenFile(path, version string) (io.ReadSeekCloser, error) {
	if version == "" {
		return os.Open(r.Abs(path))
	}
	id, err := r.Resolve(version)
	if err != nil {
		return nil, err
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, err
	}
	f, ok := m.FileMap()[path]
	if !ok {
		return nil, fmt.Errorf("%s is not in that version: %w", path, fs.ErrNotExist)
	}
	if err := r.fetchFile(f); err != nil {
		return nil, err
	}
	return r.openObject(f.Hash)
}

// RestoreFile puts one file back as it is in a version ("" for the version
// the project is on: discards its changes). A file the version does not have
// is removed. Samples in a restored set are relinked for this computer.
func (r *Repo) RestoreFile(path, version string) error { return r.RestoreFileFrom(path, path, version) }

// RestoreFileFrom puts the file source of a version at path (the file had
// another name or place then).
func (r *Repo) RestoreFileFrom(path, source, version string) error {
	if version == "" {
		version = r.Head()
	}
	if version == "" {
		return errors.New("no version yet")
	}
	id, err := r.Resolve(version)
	if err != nil {
		return err
	}
	m, err := r.Load(id)
	if err != nil {
		return err
	}
	f, ok := m.FileMap()[source]
	if !ok {
		if err := store.Remove(r.Abs(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		r.removeEmptyFolders(path)
		return nil
	}
	f.Path = path
	if err := r.fetchFile(f); err != nil {
		return err
	}
	if err := r.exportObject(f.Hash, r.Abs(path)); err != nil {
		return err
	}
	ix := r.loadIndex()
	if err := ix.record(r.Abs(path), f.Path, f.Hash, f.Size); err != nil {
		return err
	}
	if isSet(path) {
		one := *m
		one.Files = []FileEntry{f}
		if _, err := r.relink(&one, ix); err != nil {
			return err
		}
	}
	return ix.save()
}
