package project

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"dawgit/internal/als"
	"dawgit/internal/manifest"
	"dawgit/internal/store"
)

type (
	FileEntry = manifest.FileEntry
	Manifest  = manifest.Manifest
)

func (r *Repo) snapshotPath(id string) string { return filepath.Join(r.Dir, "snapshots", id+".json") }

// Head returns the current snapshot id, or "" before the first snapshot.
func (r *Repo) Head() string {
	b, err := os.ReadFile(filepath.Join(r.Dir, "HEAD"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (r *Repo) setHead(id string) error {
	return os.WriteFile(filepath.Join(r.Dir, "HEAD"), []byte(id+"\n"), 0o644)
}

// Resolve expands a unique id prefix, "HEAD" or "HEAD~N" to a full snapshot id.
func (r *Repo) Resolve(ref string) (string, error) {
	if rest, ok := strings.CutPrefix(ref, "HEAD~"); ok {
		n := 1
		if rest != "" {
			if _, err := fmt.Sscanf(rest, "%d", &n); err != nil {
				return "", fmt.Errorf("bad ref %q", ref)
			}
		}
		id, err := r.Resolve("HEAD")
		for ; err == nil && n > 0; n-- {
			var m *Manifest
			if m, err = r.Load(id); err == nil {
				if len(m.Parents) == 0 {
					return "", fmt.Errorf("%s: history is not that long", ref)
				}
				id = m.Parents[0]
			}
		}
		return id, err
	}
	if ref == "HEAD" || ref == "" {
		if h := r.Head(); h != "" {
			return h, nil
		}
		return "", errors.New("no snapshots yet")
	}
	entries, err := os.ReadDir(filepath.Join(r.Dir, "snapshots"))
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if strings.HasPrefix(id, ref) {
			found = append(found, id)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("unknown snapshot %q", ref)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("ambiguous snapshot %q (%d matches)", ref, len(found))
}

func (r *Repo) Load(id string) (*Manifest, error) {
	data, err := os.ReadFile(r.snapshotPath(id))
	if err != nil {
		return nil, err
	}
	m, err := manifest.Parse(id, data)
	if err == nil {
		headers.put(m)
	}
	return m, err
}

// Header is a version without its lists of files (Files, External are
// nil): enough for history and ancestry. Versions never change, so headers
// are kept once read; a project with many files then lists its history
// without reading every version's file list each time.
func (r *Repo) Header(id string) (*Manifest, error) {
	if h := headers.get(id); h != nil {
		return h, nil
	}
	m, err := r.Load(id)
	if err != nil {
		return nil, err
	}
	return headers.get(m.ID), nil
}

var headers = &headerCache{m: map[string]*Manifest{}}

type headerCache struct {
	mu sync.Mutex
	m  map[string]*Manifest // by version id (a content hash)
}

func (c *headerCache) get(id string) *Manifest {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := c.m[id]
	if h == nil {
		return nil
	}
	cp := *h
	cp.Parents = slices.Clone(h.Parents)
	return &cp
}

func (c *headerCache) put(m *Manifest) {
	h := *m
	h.Files, h.External = nil, nil
	h.Parents = slices.Clone(m.Parents)
	h.Packs, h.Missing = slices.Clone(m.Packs), slices.Clone(m.Missing)
	c.mu.Lock()
	c.m[m.ID] = &h
	c.mu.Unlock()
}

// HasSnapshot reports whether a snapshot is stored locally.
func (r *Repo) HasSnapshot(id string) bool {
	_, err := os.Stat(r.snapshotPath(id))
	return err == nil
}

// save seals m (computing its id) and stores it.
func (r *Repo) save(m *Manifest) error {
	data := m.Seal() // sets m.ID
	return r.storeSnapshot(m.ID, data)
}

// storeSnapshot writes an encoded manifest after checking its id.
func (r *Repo) storeSnapshot(id string, data []byte) error {
	if _, err := manifest.Parse(id, data); err != nil {
		return err
	}
	// Atomic: team state is read without the project lock.
	return store.WriteAtomic(r.snapshotPath(id), bytes.NewReader(data))
}

// sampleRefs classifies the samples referenced by the sets among files.
func (r *Repo) sampleRefs(files []FileEntry) (external []FileEntry, packs, missing []string, err error) {
	seenExt, seenPack, seenMissing := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range files {
		if !isSet(f.Path) {
			continue
		}
		s, err := als.Load(r.Abs(f.Path))
		if err != nil {
			return nil, nil, nil, err
		}
		for _, ref := range s.SampleRefs() {
			if hash, ok := r.cacheHash(ref.Path); ok {
				// Relinked into an external cache (Live may since have saved it
				// as project-relative): keep it external under its original key.
				// Samples with the same content share one cached copy: keep
				// every original that maps to it.
				for _, orig := range r.originalExternals(hash, ref.Path) {
					if !seenExt[orig.Path] {
						seenExt[orig.Path] = true
						external = append(external, orig)
					}
				}
				continue
			}
			switch {
			case ref.Pack != "":
				if !seenPack[ref.Pack] {
					seenPack[ref.Pack] = true
					packs = append(packs, ref.Pack)
				}
			case ref.RelativePathType == "3":
				// Project-relative: covered by the file scan when present.
				if _, err := os.Stat(r.Abs(ref.RelativePath)); err != nil && !seenMissing[ref.RelativePath] {
					seenMissing[ref.RelativePath] = true
					missing = append(missing, ref.RelativePath)
				}
			case ref.Path != "":
				if r.inProject(ref.Path) || seenExt[ref.Path] {
					continue
				}
				if _, err := os.Stat(filepath.FromSlash(ref.Path)); err != nil {
					if !seenMissing[ref.Path] {
						seenMissing[ref.Path] = true
						missing = append(missing, ref.Path)
					}
					continue
				}
				seenExt[ref.Path] = true
				h, n, err := r.Store.PutFile(filepath.FromSlash(ref.Path))
				if err != nil {
					return nil, nil, nil, err
				}
				external = append(external, FileEntry{Path: ref.Path, Hash: h, Size: n})
			}
		}
	}
	sort.Slice(external, func(i, j int) bool { return external[i].Path < external[j].Path })
	sort.Strings(packs)
	sort.Strings(missing)
	return external, packs, missing, nil
}

// originalExternals finds the original entries of an external sample by hash
// in HEAD's manifest (all of them: samples with the same content share one
// cached copy), falling back to fallback.
func (r *Repo) originalExternals(hash, fallback string) []FileEntry {
	var out []FileEntry
	if head := r.Head(); head != "" {
		if m, err := r.Load(head); err == nil {
			for _, e := range m.External {
				if e.Hash == hash {
					out = append(out, e)
				}
			}
		}
	}
	if len(out) == 0 {
		e := FileEntry{Path: fallback, Hash: hash}
		if fi, err := os.Stat(r.localCopy(hash)); err == nil {
			e.Size = fi.Size()
		}
		out = []FileEntry{e}
	}
	return out
}

func (r *Repo) inProject(abs string) bool {
	rel, err := filepath.Rel(r.Root, filepath.FromSlash(abs))
	return err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func isSet(rel string) bool { return strings.EqualFold(filepath.Ext(rel), ".als") }

// ErrNothingToSnapshot is returned when the working files match HEAD.
var ErrNothingToSnapshot = errors.New("nothing changed since the last snapshot")

// Snapshot records the working files as a new snapshot on top of HEAD.
func (r *Repo) Snapshot(message string) (*Manifest, error) {
	if err := r.guardLatest(); err != nil {
		return nil, err
	}
	if err := r.checkProfile(); err != nil {
		return nil, err
	}
	ix := r.loadIndex()
	r.report(StageScanning, 0, 0)
	files, err := r.workingFiles(ix)
	if err != nil {
		return nil, err
	}
	// Content the version we're on already has is kept (here or in the
	// team's storage): only new content is looked for in the store.
	var prev *Manifest
	known := map[string]bool{}
	head := r.Head()
	if head != "" {
		if prev, err = r.Load(head); err != nil {
			return nil, err
		}
		for _, f := range prev.Files {
			known[f.Hash] = true
		}
	}
	var toStore []FileEntry
	for _, f := range files {
		if !known[f.Hash] && !r.Store.Has(f.Hash) && !r.remoteOnly()[f.Hash] {
			toStore = append(toStore, f)
		}
	}
	paths := make([]string, len(toStore))
	for i, f := range toStore {
		paths[i] = f.Path
	}
	var mu sync.Mutex
	stored := 0
	if len(paths) > 0 {
		r.report(StageStoring, 0, len(paths))
	}
	if err := inParallel(paths, func(p string) error {
		if _, _, err := r.Store.PutFile(r.Abs(p)); err != nil {
			return err
		}
		mu.Lock()
		stored++
		r.report(StageStoring, stored, len(paths))
		mu.Unlock()
		return nil
	}); err != nil {
		return nil, err
	}
	external, packs, missing, err := r.sampleRefs(files)
	if err != nil {
		return nil, err
	}
	authorID, author := r.Identity()
	m := &Manifest{Version: 1, Parents: []string{}, Author: author, AuthorID: authorID,
		Time: time.Now().UTC().Format(time.RFC3339), Message: message,
		Files: files, External: external, Packs: packs, Missing: missing}
	if prev != nil {
		if sameContent(prev, m) {
			return nil, ErrNothingToSnapshot
		}
		m.Parents = []string{head}
	}
	if err := r.save(m); err != nil {
		return nil, err
	}
	if err := r.setHead(m.ID); err != nil {
		return nil, err
	}
	return m, ix.save()
}

func sameContent(a, b *Manifest) bool {
	return slices.Equal(a.Files, b.Files) && slices.Equal(a.External, b.External)
}

// Log returns every version of the current branch (everyone's, including
// both sides of merges), newest first; a version is always listed before its
// parents. Versions come as headers (see Header).
func (r *Repo) Log() ([]*Manifest, error) {
	return r.LogAll(nil)
}

// LogAll is Log for the whole tree: also every version leading to heads
// (e.g. the latest version of each branch). Heads not on this computer are
// skipped. Versions come as headers (see Header): Load one for its files.
func (r *Repo) LogAll(heads []string) ([]*Manifest, error) {
	all := map[string]*Manifest{}
	for _, h := range append([]string{r.Latest(), r.Head()}, heads...) {
		if h == "" || all[h] != nil {
			continue
		}
		if _, err := r.Header(h); err != nil {
			continue
		}
		anc, err := r.ancestors(h)
		if err != nil {
			return nil, err
		}
		for id := range anc {
			if all[id] == nil {
				m, err := r.Header(id)
				if err != nil {
					return nil, err
				}
				all[id] = m
			}
		}
	}
	children := map[string]int{} // unlisted children per version
	for _, m := range all {
		for _, p := range m.Parents {
			children[p]++
		}
	}
	var out []*Manifest
	for len(all) > 0 {
		// Among versions whose children are all listed, take the newest.
		var next *Manifest
		for _, m := range all {
			if children[m.ID] == 0 && (next == nil || m.Time > next.Time || (m.Time == next.Time && m.ID > next.ID)) {
				next = m
			}
		}
		out = append(out, next)
		delete(all, next.ID)
		for _, p := range next.Parents {
			children[p]--
		}
	}
	return out, nil
}
