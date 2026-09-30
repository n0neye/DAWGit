package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"dawgit/internal/diff"
)

// ProjectFile is a file in the project folder as the app lists it.
type ProjectFile struct {
	Path   string // slash separated, relative to the project folder
	Status string // "added" | "modified" | "deleted" | "unchanged" | "ignored"
	Size   int64  // on disk (0 for deleted files)
}

// Files lists the changed files; with all, also unchanged files and the ones
// DAWGit leaves out of versions (Live's backups and caches).
func (r *Repo) Files(all bool) ([]ProjectFile, error) {
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	for _, c := range changes {
		status[c.Path] = c.Status
	}
	var out []ProjectFile
	for _, c := range changes {
		if c.Status == "deleted" {
			out = append(out, ProjectFile{Path: c.Path, Status: "deleted"})
		}
	}
	err = filepath.WalkDir(r.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == r.Root {
			return err
		}
		rel, _ := filepath.Rel(r.Root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == metaDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		st, changed := status[rel]
		switch {
		case changed:
		case !all:
			return nil
		case ignoredPath(rel):
			st = "ignored"
		default:
			st = "unchanged"
		}
		var size int64
		if fi, err := d.Info(); err == nil {
			size = fi.Size()
		}
		out = append(out, ProjectFile{Path: rel, Status: st, Size: size})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}

// IgnoredPath: the file or one of its folders is left out of versions.
func IgnoredPath(rel string) bool { return ignoredPath(rel) }

func ignoredPath(rel string) bool {
	if Ignored(rel, false) {
		return true
	}
	for dir := filepath.ToSlash(filepath.Dir(rel)); dir != "." && dir != "/"; dir = filepath.ToSlash(filepath.Dir(dir)) {
		if Ignored(dir, true) {
			return true
		}
	}
	return false
}

// FileVersion is a version that changed a file.
type FileVersion struct {
	Version *Manifest
	Status  string // "added" | "modified" | "deleted"
	Hash    string // the file's content in that version ("" when deleted)
}

// FileHistory lists the versions of the current branch (newest first) that
// added, changed or deleted path.
func (r *Repo) FileHistory(path string) ([]FileVersion, error) {
	log, err := r.Log()
	if err != nil {
		return nil, err
	}
	var out []FileVersion
	for _, m := range log {
		cur := m.FileMap()[path].Hash
		prev := ""
		if len(m.Parents) > 0 {
			if p, err := r.Load(m.Parents[0]); err == nil {
				prev = p.FileMap()[path].Hash
			}
		}
		switch {
		case cur == prev:
			continue
		case prev == "":
			out = append(out, FileVersion{Version: m, Status: "added", Hash: cur})
		case cur == "":
			out = append(out, FileVersion{Version: m, Status: "deleted"})
		default:
			out = append(out, FileVersion{Version: m, Status: "modified", Hash: cur})
		}
	}
	return out, nil
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
	if r.Store.Has(f.Hash) {
		return nil
	}
	c, err := r.Client()
	if err != nil {
		return fmt.Errorf("this version of %s is not on this computer: %w", f.Path, err)
	}
	r.knowSizes(&Manifest{Files: []FileEntry{f}})
	return r.fetchObjects(c, []string{f.Hash})
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
		return nil, fmt.Errorf("%s is not in that version", path)
	}
	if err := r.fetchFile(f); err != nil {
		return nil, err
	}
	return os.Open(r.Store.Path(f.Hash))
}

// RestoreFile puts one file back as it is in a version ("" for the version
// the project is on: discards its changes). A file the version does not have
// is removed. Samples in a restored set are relinked for this computer.
func (r *Repo) RestoreFile(path, version string) error {
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
	f, ok := m.FileMap()[path]
	if !ok {
		if err := os.Remove(r.Abs(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := r.fetchFile(f); err != nil {
		return err
	}
	if err := r.Store.Export(f.Hash, r.Abs(path)); err != nil {
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
