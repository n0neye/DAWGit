package remote

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"dawgit/internal/blob"
	"dawgit/internal/chunk"
	"dawgit/internal/manifest"
)

// Cleaning up a team's storage: files no version of any project uses (left
// by deleted projects, or by uploads that stopped) are deleted. Deleting
// shared data must never remove a file someone is about to use:
//
//   - A client sharing versions first writes a lease (gc/leases/<id>)
//     listing every file and folder list it relies on, and removes it when
//     done. Leased files are kept; leases older than leaseLife are leftovers.
//   - Files written in the last minAge are kept: a share uploads files
//     before the version that uses them.
//   - Unused files are first marked (gc/candidates.json), and deleted only
//     by a cleanup at least confirmAfter later, if still unused.
//   - A big file may be kept as pieces (a chunk list, marked chunked/<hash>):
//     its pieces are used for as long as the list is stored, used or not, so
//     a share relying on a list that is there keeps its pieces too. Pieces
//     of a deleted list go in a later cleanup.

const (
	leaseLife    = 14 * 24 * time.Hour
	minAge       = 7 * 24 * time.Hour
	confirmAfter = 24 * time.Hour

	leasesDir      = "gc/leases/"
	candidatesKey  = "gc/candidates.json"
	gcObjectsDir   = "objects/"
	gcProjectsDir  = "projects/"
	gcSnapshotsDir = "snapshots/"
	chunkedDir     = "chunked/"
)

var gcNow = time.Now // tests move time on

// Leaser is a backend whose cleanup can be told which files a share relies
// on (see CollectGarbage).
type Leaser interface {
	// Lease keeps the files (content or tree hashes) until release.
	Lease(hashes []string) (release func(), err error)
}

var _ Leaser = (*BucketBackend)(nil)

func (s *BucketBackend) Lease(hashes []string) (func(), error) {
	id := make([]byte, 12)
	rand.Read(id)
	key := leasesDir + hex.EncodeToString(id)
	if err := s.put(key, []byte(strings.Join(hashes, "\n"))); err != nil {
		return nil, err
	}
	return func() { s.delete(key) }, nil
}

// GCReport is what a cleanup found and did.
type GCReport struct {
	Versions int // version records read (all projects)
	Used     int // files some version (or a share in progress) uses
	Stored   int // files in storage
	// Unused files: Due for deleting (deleted, with remove); Waiting to be
	// confirmed by a later cleanup (or too recent to judge).
	Due, Deleted, Waiting                int
	DueBytes, DeletedBytes, WaitingBytes int64
	// NextCleanup: when waiting files can be deleted (zero when none).
	NextCleanup time.Time
}

// CollectGarbage finds the files no version uses and marks them; with
// remove it also deletes those marked at least a day before.
func (s *BucketBackend) CollectGarbage(remove bool) (*GCReport, error) {
	now := gcNow()
	rep := &GCReport{}
	used, err := s.usedFiles(rep)
	if err != nil {
		return nil, err
	}
	// Leases: files shares in progress rely on.
	var stale []string
	leases, err := listAll(s.b, leasesDir)
	if err != nil {
		return nil, err
	}
	for _, l := range leases {
		if now.Sub(l.Modified) > leaseLife {
			stale = append(stale, l.Key)
			continue
		}
		data, err := s.get(l.Key)
		if errors.Is(err, ErrNotFound) {
			continue // released meanwhile
		}
		if err != nil {
			return nil, err
		}
		for _, h := range strings.Fields(string(data)) {
			used[h] = true
		}
	}
	rep.Used = len(used)

	stored, err := listAll(s.b, gcObjectsDir)
	if err != nil {
		return nil, err
	}
	rep.Stored = len(stored)
	isStored := map[string]bool{}
	for _, it := range stored {
		isStored[strings.ReplaceAll(strings.TrimPrefix(it.Key, gcObjectsDir), "/", "")] = true
	}
	// Pieces of the files kept as chunk lists.
	markers, err := listAll(s.b, chunkedDir)
	if err != nil {
		return nil, err
	}
	var lists, staleMarkers []string
	for _, m := range markers {
		h := strings.TrimPrefix(m.Key, chunkedDir)
		switch {
		case isStored[h]:
			lists = append(lists, h)
		case now.Sub(m.Modified) >= minAge: // the list was deleted (or never made it)
			staleMarkers = append(staleMarkers, m.Key)
		}
	}
	var mu sync.Mutex
	err = parallelN(checks, lists, func(h string) error {
		l, err := s.chunkList(h)
		if err != nil {
			return fmt.Errorf("a big file's list of pieces %s can't be read (%w): storage not cleaned up", h[:10], err)
		}
		mu.Lock()
		defer mu.Unlock()
		for _, p := range l.Pieces {
			used[p.Hash] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	rep.Used = len(used)
	marked := map[string]time.Time{} // hash: first found unused
	if data, err := s.get(candidatesKey); err == nil {
		json.Unmarshal(data, &marked)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	nextMarked := map[string]time.Time{}
	var doomed []string
	for _, it := range stored {
		h := strings.ReplaceAll(strings.TrimPrefix(it.Key, gcObjectsDir), "/", "")
		if used[h] || len(h) != 64 {
			continue
		}
		since, ok := marked[h]
		if !ok {
			since = now
		}
		if now.Sub(it.Modified) >= minAge && now.Sub(since) >= confirmAfter {
			rep.Due++
			rep.DueBytes += it.Size
			if remove {
				doomed = append(doomed, it.Key)
				rep.Deleted++
				rep.DeletedBytes += it.Size
				continue
			}
		}
		nextMarked[h] = since
		rep.Waiting++
		rep.WaitingBytes += it.Size
		ready := since.Add(confirmAfter)
		if old := it.Modified.Add(minAge); old.After(ready) {
			ready = old
		}
		if rep.NextCleanup.IsZero() || ready.Before(rep.NextCleanup) {
			rep.NextCleanup = ready
		}
	}
	if remove {
		if err := parallelN(checks, doomed, s.delete); err != nil {
			return nil, err
		}
		for _, l := range stale {
			s.delete(l)
		}
		for _, m := range staleMarkers {
			s.delete(m)
		}
	}
	data, _ := json.Marshal(nextMarked)
	if err := s.put(candidatesKey, data); err != nil {
		return nil, err
	}
	return rep, nil
}

// usedFiles reads every version record of every project (also projects
// being created or deleted) and the folder lists they name.
func (s *BucketBackend) usedFiles(rep *GCReport) (map[string]bool, error) {
	dirs, err := s.b.Folders(gcProjectsDir)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	used := map[string]bool{}
	var roots []string
	var keys []string
	for _, dir := range dirs {
		ks, err := s.list(dir + gcSnapshotsDir)
		if err != nil {
			return nil, err
		}
		keys = append(keys, ks...)
	}
	err = parallelN(checks, keys, func(key string) error {
		data, err := s.get(key)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(key[strings.LastIndex(key, "/")+1:], ".json")
		m, err := manifest.Parse(id, data)
		if err != nil {
			// Can't tell what it uses: cleaning up could delete its files.
			return fmt.Errorf("version %s can't be read (%w): storage not cleaned up", id[:min(10, len(id))], err)
		}
		mu.Lock()
		defer mu.Unlock()
		rep.Versions++
		for _, h := range m.Objects() {
			used[h] = true
		}
		if m.Tree != "" {
			roots = append(roots, m.Tree)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Folder lists, a level at a time: each is read once.
	level := roots
	for len(level) > 0 {
		var todo []string
		for _, h := range level {
			if !used[h] {
				used[h] = true
				todo = append(todo, h)
			}
		}
		var next []string
		err := parallelN(checks, todo, func(h string) error {
			data, err := s.get(objectKey(h))
			if err != nil {
				return fmt.Errorf("a version's folder list %s can't be read (%w): storage not cleaned up", h[:10], err)
			}
			entries, err := manifest.ParseTree(h, data)
			if err != nil {
				return fmt.Errorf("%w: storage not cleaned up", err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, e := range entries {
				if e.Dir {
					next = append(next, e.Hash)
				} else {
					used[e.Hash] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		level = next
	}
	return used, nil
}

// chunkList reads the list of pieces stored for the file h.
func (s *BucketBackend) chunkList(h string) (*chunk.List, error) {
	data, err := s.get(objectKey(h))
	if err != nil {
		return nil, err
	}
	rc, list, err := blob.Open(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	if !list {
		return &chunk.List{}, nil // kept whole after all
	}
	text, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return chunk.Parse(text)
}
