package project

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/store"
)

// Ignored reports whether a project-relative path (slash separated) is left
// out of snapshots: Live's own backups and analysis caches, OS litter.
func Ignored(rel string, isDir bool) bool {
	base := rel[strings.LastIndex(rel, "/")+1:]
	if isDir {
		return rel == metaDir || rel == "Backup" || base == ".git"
	}
	switch strings.ToLower(base) {
	case "desktop.ini", "thumbs.db", ".ds_store":
		return true
	}
	return strings.HasSuffix(strings.ToLower(base), ".asd") || strings.HasPrefix(base, ".dawgit-")
}

// scan lists working files as slash-separated paths relative to the root.
func (r *Repo) scan() ([]string, error) {
	var files []string
	err := filepath.WalkDir(r.Root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == r.Root {
			return nil
		}
		rel, _ := filepath.Rel(r.Root, p)
		rel = filepath.ToSlash(rel)
		if Ignored(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// index caches file stat -> content hash, so unchanged samples are not
// rehashed, and so a file rewritten on checkout (sample relinking) still maps
// to the snapshot object it came from.
type indexEntry struct {
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Hash  string `json:"hash"`
}

type index struct {
	path    string
	entries map[string]indexEntry
	dirty   bool
}

func (r *Repo) loadIndex() *index {
	ix := &index{path: filepath.Join(r.Dir, "index.json"), entries: map[string]indexEntry{}}
	_ = readJSON(ix.path, &ix.entries)
	return ix
}

func (ix *index) save() error {
	if !ix.dirty {
		return nil
	}
	return writeJSON(ix.path, ix.entries)
}

// record associates the file's current stat with hash.
func (ix *index) record(abs, rel, hash string) error {
	fi, err := os.Stat(abs)
	if err != nil {
		return err
	}
	ix.entries[rel] = indexEntry{fi.Size(), fi.ModTime().UnixNano(), hash}
	ix.dirty = true
	return nil
}

// hash returns the content hash of a working file, using the stat cache.
func (ix *index) hash(abs, rel string) (string, int64, error) {
	fi, err := os.Stat(abs)
	if err != nil {
		return "", 0, err
	}
	if e, ok := ix.entries[rel]; ok && e.Size == fi.Size() && e.Mtime == fi.ModTime().UnixNano() {
		return e.Hash, e.Size, nil
	}
	h, n, err := store.HashFile(abs)
	if err != nil {
		return "", 0, err
	}
	ix.entries[rel] = indexEntry{fi.Size(), fi.ModTime().UnixNano(), h}
	ix.dirty = true
	return h, n, nil
}

// workingFiles hashes every tracked working file.
func (r *Repo) workingFiles(ix *index) ([]FileEntry, error) {
	paths, err := r.scan()
	if err != nil {
		return nil, err
	}
	out := make([]FileEntry, 0, len(paths))
	for _, rel := range paths {
		h, n, err := ix.hash(r.Abs(rel), rel)
		if err != nil {
			return nil, err
		}
		out = append(out, FileEntry{Path: rel, Hash: h, Size: n})
	}
	return out, nil
}
