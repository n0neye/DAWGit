package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	return manifest.Parse(id, data)
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
				orig := r.originalExternalPath(hash, ref.Path)
				if !seenExt[orig] {
					seenExt[orig] = true
					fi, _ := os.Stat(r.Store.Path(hash))
					external = append(external, FileEntry{Path: orig, Hash: hash, Size: fi.Size()})
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

// originalExternalPath finds the original location of an external sample
// by hash in HEAD's manifest, falling back to fallback.
func (r *Repo) originalExternalPath(hash, fallback string) string {
	if head := r.Head(); head != "" {
		if m, err := r.Load(head); err == nil {
			for _, e := range m.External {
				if e.Hash == hash {
					return e.Path
				}
			}
		}
	}
	return fallback
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
	ix := r.loadIndex()
	r.report(StageScanning, 0, 0)
	files, err := r.workingFiles(ix)
	if err != nil {
		return nil, err
	}
	var toStore []FileEntry
	for _, f := range files {
		if !r.Store.Has(f.Hash) {
			toStore = append(toStore, f)
		}
	}
	for i, f := range toStore {
		r.report(StageStoring, i, len(toStore))
		if _, _, err := r.Store.PutFile(r.Abs(f.Path)); err != nil {
			return nil, err
		}
	}
	external, packs, missing, err := r.sampleRefs(files)
	if err != nil {
		return nil, err
	}
	authorID, author := r.Identity()
	m := &Manifest{Version: 1, Parents: []string{}, Author: author, AuthorID: authorID,
		Time: time.Now().UTC().Format(time.RFC3339), Message: message,
		Files: files, External: external, Packs: packs, Missing: missing}
	if head := r.Head(); head != "" {
		prev, err := r.Load(head)
		if err != nil {
			return nil, err
		}
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
	enc := func(m *Manifest) []byte {
		data, _ := json.Marshal([]any{m.Files, m.External})
		return data
	}
	return bytes.Equal(enc(a), enc(b))
}

// Log returns every version of the current branch (everyone's, including
// both sides of merges), newest first; a version is always listed before its
// parents.
func (r *Repo) Log() ([]*Manifest, error) {
	return r.LogAll(nil)
}

// LogAll is Log for the whole tree: also every version leading to heads
// (e.g. the latest version of each branch). Heads not on this computer are
// skipped.
func (r *Repo) LogAll(heads []string) ([]*Manifest, error) {
	all := map[string]*Manifest{}
	for _, h := range append([]string{r.Latest(), r.Head()}, heads...) {
		if h == "" || all[h] != nil {
			continue
		}
		if _, err := r.Load(h); err != nil {
			continue
		}
		anc, err := r.ancestors(h)
		if err != nil {
			return nil, err
		}
		for id := range anc {
			if all[id] == nil {
				m, err := r.Load(id)
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
