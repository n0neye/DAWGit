package project

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/manifest"
	"dawgit/internal/profile"
	"dawgit/internal/store"
)

// Where file contents live.
//
// A project kept on this computer only keeps every version's files in its
// object store. A team project keeps its sets there (they are small, and
// diffs and merges read them), but once samples and other files are safely
// in the team's storage their local copies are removed (PruneObjects): the
// project folder already has the current ones, and older ones are downloaded
// when a version needs them. The hashes removed this way are listed in
// .dawgit/remote-objects, so a commit doesn't copy unchanged files in again.

const remoteObjectsFile = "remote-objects"

// ErrNotHere: a version needs files that are only in the team's storage, and
// the project is not in a team (e.g. kept under Local after disconnecting).
var ErrNotHere = errors.New("some files of this version are only in the team's storage: join the team again to use it")

// remoteOnly lists objects known to be in the team's storage and not kept here.
func (r *Repo) remoteOnly() map[string]bool {
	if r.remote != nil {
		return r.remote
	}
	r.remote = map[string]bool{}
	f, err := os.Open(filepath.Join(r.Dir, remoteObjectsFile))
	if err != nil {
		return r.remote
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if h := strings.TrimSpace(sc.Text()); h != "" {
			r.remote[h] = true
		}
	}
	return r.remote
}

func (r *Repo) saveRemoteOnly() error {
	var hs []string
	for h := range r.remoteOnly() {
		hs = append(hs, h)
	}
	sort.Strings(hs)
	return store.WriteAtomic(filepath.Join(r.Dir, remoteObjectsFile), strings.NewReader(strings.Join(hs, "\n")+"\n"))
}

// ForgetRemoteObjects drops the list of objects kept only in the team's
// storage, once every one of them is here again (see DownloadHistory).
func (r *Repo) ForgetRemoteObjects() error {
	r.remote = map[string]bool{}
	err := os.Remove(filepath.Join(r.Dir, remoteObjectsFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// sourcesByHash maps contents found on this computer outside the object
// store to a file holding them: external samples materialized in the cache,
// and project files whose content is unchanged since they were hashed (not
// sets: a relinked set's content differs from the version it came from).
func (r *Repo) sourcesByHash() map[string]string {
	if r.sources != nil {
		return r.sources
	}
	r.sources = map[string]string{}
	r.stamps = map[string]stamp{}
	ix := r.loadIndex()
	for rel, e := range ix.entries {
		if isSet(rel) || (e.ObjSize != 0 && e.ObjSize != e.Size) {
			continue
		}
		abs := r.Abs(rel)
		if fi, err := os.Stat(abs); err == nil && fi.Size() == e.Size && fi.ModTime().UnixNano() == e.Mtime {
			r.sources[e.Hash] = abs
			r.stamps[abs] = stamp{e.Size, e.Mtime}
		}
	}
	if dirs, err := os.ReadDir(filepath.Join(r.Dir, externalDir)); err == nil {
		for _, d := range dirs {
			files, _ := os.ReadDir(filepath.Join(r.Dir, externalDir, d.Name()))
			for _, f := range files {
				if f.Type().IsRegular() {
					r.sources[d.Name()] = filepath.Join(r.Dir, externalDir, d.Name(), f.Name())
					break
				}
			}
		}
	}
	return r.sources
}

// stamp is a project file's size and modification time when it was hashed:
// while they are the same, so is its content.
type stamp struct{ size, mtime int64 }

// localCopy is a file on this computer with the content h ("" if none). A
// project file counts only while it is as it was hashed: one changed since
// (overwritten, edited) never stands in for the content it had.
func (r *Repo) localCopy(h string) string {
	if r.Store.Has(h) {
		return r.Store.Path(h)
	}
	p := r.sourcesByHash()[h]
	if p == "" {
		return ""
	}
	fi, err := os.Stat(p)
	if err != nil {
		delete(r.sources, h)
		return ""
	}
	if st, ok := r.stamps[p]; ok && (fi.Size() != st.size || fi.ModTime().UnixNano() != st.mtime) {
		delete(r.sources, h)
		return ""
	}
	return p
}

// copyHere is a local copy of h, downloading it from the team when there is
// none (any more): a project file may have been the only one here.
func (r *Repo) copyHere(h string) (string, error) {
	if p := r.localCopy(h); p != "" {
		return p, nil
	}
	if r.Config.Remote == nil {
		return "", fmt.Errorf("object %s is not on this computer", short(h))
	}
	if err := r.ensureHashes([]string{h}); err != nil {
		return "", fmt.Errorf("object %s: %w", short(h), err)
	}
	if p := r.localCopy(h); p != "" {
		return p, nil
	}
	return "", fmt.Errorf("object %s is not on this computer", short(h))
}

// available: the content h can be had without downloading it.
func (r *Repo) available(h string) bool { return r.localCopy(h) != "" }

func (r *Repo) openObject(h string) (*os.File, error) {
	p, err := r.copyHere(h)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// exportObject writes the content h to dst (from the store, or a file here
// with the same content).
func (r *Repo) exportObject(h, dst string) error {
	if r.Store.Has(h) {
		return r.Store.Export(h, dst)
	}
	src, err := r.copyHere(h)
	if err != nil {
		return err
	}
	if r.Store.Has(h) { // downloaded just now
		return r.Store.Export(h, dst)
	}
	if same, _ := filepath.Abs(dst); strings.EqualFold(same, src) {
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	return store.WriteAtomic(dst, f)
}

// ensureHashes makes contents available here, downloading the missing ones
// from the team.
func (r *Repo) ensureHashes(hashes []string) error {
	var need []string
	for _, h := range dedupe(hashes) {
		if !r.available(h) {
			need = append(need, h)
		}
	}
	if len(need) == 0 {
		return nil
	}
	if r.Config.Remote == nil {
		return ErrNotHere
	}
	c, err := r.Client()
	if err != nil {
		return fmt.Errorf("some files of this version are not on this computer: %w", err)
	}
	return r.fetchObjects(c, need)
}

// allManifests reads every version stored here, with its files.
func (r *Repo) allManifests() ([]*Manifest, error) {
	ids, err := r.storedVersions()
	if err != nil {
		return nil, err
	}
	var out []*Manifest
	for _, id := range ids {
		if m, err := r.Load(id); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// storedVersions lists the ids of the versions stored here.
func (r *Repo) storedVersions() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(r.Dir, "snapshots"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if id, ok := strings.CutSuffix(e.Name(), ".json"); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// referenced lists every object a version here refers to; sets marks those
// kept on this computer anyway: Live Sets, and the rules (.dawgit.yaml) that
// decide what an update may delete. Versions share most folders: each
// folder's list is read once.
func (r *Repo) referenced() (all map[string]int64, sets map[string]bool, err error) {
	ids, err := r.storedVersions()
	if err != nil {
		return nil, nil, err
	}
	all, sets = map[string]int64{}, map[string]bool{}
	add := func(path string, f FileEntry) {
		all[f.Hash] = f.Size
		if isSet(path) || path == profile.FileName {
			sets[f.Hash] = true
		}
	}
	tops := map[string]bool{}
	for _, id := range ids {
		m, err := r.readRecord(id)
		if err != nil {
			continue
		}
		for _, f := range m.Files { // format 1
			add(f.Path, f)
		}
		for _, f := range m.External {
			all[f.Hash] = f.Size
		}
		if m.Tree != "" {
			tops[m.Tree] = true
		}
	}
	var roots []string
	for h := range tops {
		roots = append(roots, h)
	}
	err = r.walkTrees(roots, func(h string, entries []manifest.TreeEntry) error {
		for _, e := range entries {
			if e.Dir {
				continue
			}
			name := e.Name // a set by its name; the rules file only at the top
			if !tops[h] && name == profile.FileName {
				name = "sub/" + name
			}
			add(name, FileEntry{Hash: e.Hash, Size: e.Size})
		}
		return nil
	})
	return all, sets, err
}

// PruneObjects removes local copies of a team project's samples and other
// files (not sets) once the team's storage has them, and returns the bytes
// freed. Nothing is removed for a project kept on this computer only.
func (r *Repo) PruneObjects() (int64, error) {
	if r.Config.Remote == nil {
		return 0, nil
	}
	all, sets, err := r.referenced()
	if err != nil {
		return 0, err
	}
	var candidates []string
	for h := range all {
		if !sets[h] && r.Store.Has(h) {
			candidates = append(candidates, h)
		}
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	c, err := r.Client()
	if err != nil {
		return 0, err
	}
	missing, err := c.MissingObjects(candidates)
	if err != nil {
		return 0, err
	}
	notThere := map[string]bool{}
	for _, h := range missing {
		notThere[h] = true
	}
	var freed int64
	remote := r.remoteOnly()
	for _, h := range candidates {
		if notThere[h] {
			continue // not uploaded yet: keep it
		}
		remote[h] = true
	}
	// Record first: a removed object must never look like a new file.
	if err := r.saveRemoteOnly(); err != nil {
		return 0, err
	}
	for _, h := range candidates {
		if notThere[h] {
			continue
		}
		if fi, err := os.Stat(r.Store.Path(h)); err == nil {
			if os.Remove(r.Store.Path(h)) == nil {
				freed += fi.Size()
			}
		}
	}
	r.sources = nil
	return freed, nil
}

// GC removes objects no version refers to (e.g. left by discarded work) and
// forgets remote-only objects nothing refers to; it returns the bytes freed.
func (r *Repo) GC() (int64, error) {
	all, _, err := r.referenced()
	if err != nil {
		return 0, err
	}
	var freed int64
	root := filepath.Dir(r.Store.Path(strings.Repeat("0", 64)))
	root = filepath.Dir(root) // the objects directory
	shards, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue // e.g. tmp
		}
		files, _ := os.ReadDir(filepath.Join(root, shard.Name()))
		for _, f := range files {
			h := shard.Name() + f.Name()
			if _, ok := all[h]; ok || len(h) != 64 {
				continue
			}
			p := filepath.Join(root, shard.Name(), f.Name())
			if fi, err := os.Stat(p); err == nil && os.Remove(p) == nil {
				freed += fi.Size()
			}
		}
	}
	remote := r.remoteOnly()
	changed := false
	for h := range remote {
		if _, ok := all[h]; !ok {
			delete(remote, h)
			changed = true
		}
	}
	if changed {
		if err := r.saveRemoteOnly(); err != nil {
			return freed, err
		}
	}
	return freed, nil
}

// HistoryNotHere is what DownloadHistory would download: files of older
// versions that are only in the team's storage (count and bytes).
func (r *Repo) HistoryNotHere() (int, int64, error) {
	all, _, err := r.referenced()
	if err != nil {
		return 0, 0, err
	}
	var n int
	var size int64
	for h, s := range all {
		if !r.available(h) {
			n++
			size += s
		}
	}
	return n, size, nil
}

// DownloadHistory downloads every file of every version that is not on this
// computer, so the project no longer needs the team's storage.
func (r *Repo) DownloadHistory() error {
	all, _, err := r.referenced()
	if err != nil {
		return err
	}
	var hs []string
	for h := range all {
		hs = append(hs, h)
	}
	if r.sizes == nil {
		r.sizes = map[string]int64{}
	}
	maps.Copy(r.sizes, all)
	return r.ensureHashes(hs)
}

// PrepareDetach makes sure a project leaving its team keeps what it needs:
// the version it is on (files changed since are restorable), and with full
// every older version too.
func (r *Repo) PrepareDetach(full bool) error {
	if full {
		if err := r.DownloadHistory(); err != nil {
			return err
		}
		return r.ForgetRemoteObjects()
	}
	if head := r.Head(); head != "" {
		m, err := r.Load(head)
		if err != nil {
			return err
		}
		r.knowSizes(m)
		return r.ensureHashes(m.Objects())
	}
	return nil
}

// MissingHere reports, for each version, whether some of its files are only
// in the team's storage while the project has no team to get them from.
func (r *Repo) MissingHere(ms []*Manifest) map[string]bool {
	out := map[string]bool{}
	if r.Config.Remote != nil || len(r.remoteOnly()) == 0 {
		return out
	}
	for _, m := range ms {
		if m.Files == nil { // a header
			full, err := r.Load(m.ID)
			if err != nil {
				continue
			}
			m = full
		}
		for _, h := range m.Objects() {
			if !r.available(h) {
				out[m.ID] = true
				break
			}
		}
	}
	return out
}

// setHashes lists the Live Sets of some versions.
func setHashes(ms ...*Manifest) []string {
	var out []string
	for _, m := range ms {
		for _, f := range m.Files {
			if isSet(f.Path) {
				out = append(out, f.Hash)
			}
		}
	}
	return out
}
