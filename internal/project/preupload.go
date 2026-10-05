package project

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/store"
)

// Big files (a video, a long recording) are put in the team's storage in
// the background once they stop changing, before they are committed: the
// commit then has nothing to copy or upload for them, and is done in a
// moment. Nothing is shared by this: storage keeps contents by their hash,
// and no version names them until a commit does.
//
//   - A file goes up when it is a change (added or modified, not a set), at
//     least PreuploadMin, and unchanged for PreuploadStable.
//   - It is copied into .dawgit/preupload first, hashed on the way: what goes
//     up is exactly what the hash says, whatever happens to the file meanwhile.
//     The copy goes once it's up.
//   - The team's cleanup keeps it: a lease, never released, kept leaseLife
//     (14 days); after that an uncommitted upload is cleaned up like any
//     other leftover.
//   - Here it is listed among the team's objects (remote-objects), so a
//     commit doesn't copy it into the history again, and in preuploaded
//     with the time, so local cleanup (GC) keeps that entry for
//     preuploadKeep, a little less than the lease.

var (
	// PreuploadMin and PreuploadStable: which files go up early (tests
	// change them).
	PreuploadMin    int64 = 50 << 20
	PreuploadStable       = 2 * time.Minute
)

const (
	preuploadDir  = "preupload"
	preuploadList = "preuploaded" // "<hash> <unix time>" per line
	preuploadKeep = 12 * 24 * time.Hour
)

// PreuploadCandidate is a file to put in the team's storage early.
type PreuploadCandidate struct {
	Path string // relative, slash separated
	Hash string
	Size int64
}

// PreuploadCandidates lists the big changed files that have stopped
// changing and the team doesn't have yet (as far as this copy knows), biggest
// first. Hashing them (once; the index keeps it) reads them.
func (r *Repo) PreuploadCandidates(now time.Time) ([]PreuploadCandidate, error) {
	if r.Config.Remote == nil || r.OnOlderVersion() {
		return nil, nil
	}
	ix := r.loadIndex()
	files, err := r.workingFiles(ix)
	if err != nil {
		return nil, err
	}
	ix.save()
	var head map[string]FileEntry
	if id := r.Head(); id != "" {
		m, err := r.Load(id)
		if err != nil {
			return nil, err
		}
		head = m.FileMap()
	}
	have := r.remoteOnly()
	var out []PreuploadCandidate
	for _, f := range files {
		if f.Size < PreuploadMin || isSet(f.Path) || have[f.Hash] || r.Store.Has(f.Hash) {
			continue
		}
		if h, ok := head[f.Path]; ok && h.Hash == f.Hash {
			continue // not a change
		}
		fi, err := os.Stat(r.Abs(f.Path))
		if err != nil || fi.Size() != f.Size || now.Sub(fi.ModTime()) < PreuploadStable {
			continue // still being written
		}
		out = append(out, PreuploadCandidate{Path: f.Path, Hash: f.Hash, Size: f.Size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out, nil
}

// ErrChangedSince: the file changed after it was found (it goes up later).
var ErrChangedSince = errors.New("the file changed: it goes up once it stops changing")

// Preupload puts a candidate in the team's storage (see the top of this
// file); progress gets bytes done and total. It needs no lock: it reads the
// file and writes only in .dawgit/preupload. NotePreuploaded then records
// it, under the project's lock.
func (r *Repo) Preupload(c remote.Backend, cand PreuploadCandidate, progress func(done, total int64)) error {
	dir := filepath.Join(r.Dir, preuploadDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// The copy, hashed on the way.
	tmp, err := os.CreateTemp(dir, "tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	src, err := os.Open(r.Abs(cand.Path))
	if err != nil {
		tmp.Close()
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(tmp, h), src)
	src.Close()
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != cand.Hash {
		return ErrChangedSince
	}
	copyPath := filepath.Join(dir, cand.Hash)
	if err := os.Rename(tmp.Name(), copyPath); err != nil {
		return err
	}
	defer os.Remove(copyPath)

	// Up, from the copy; the team's cleanup leaves it (and its pieces).
	r.pin(cand.Hash, copyPath)
	defer r.unpin(cand.Hash)
	hold := func(hashes []string) error {
		if l, ok := c.(remote.Leaser); ok {
			_, err := l.Lease(hashes) // never released: kept leaseLife
			return err
		}
		return nil
	}
	if err := hold([]string{cand.Hash}); err != nil {
		return err
	}
	onProgress := r.OnProgress
	r.OnProgress = func(p Progress) {
		if progress != nil && p.Stage == StageUploading {
			progress(p.Bytes, p.TotalBytes)
		}
	}
	defer func() { r.OnProgress = onProgress }()
	return r.uploadObjects(c, []string{cand.Hash}, hold)
}

// NotePreuploaded records that the team's storage has h (see Preupload):
// a commit then neither copies nor uploads it. Call it with the project
// locked.
func (r *Repo) NotePreuploaded(h string) error {
	r.remote = nil // as on disk now: someone else may have added to it
	remote := r.remoteOnly()
	remote[h] = true
	if err := r.saveRemoteOnly(); err != nil {
		return err
	}
	list := r.preuploaded()
	list[h] = time.Now()
	return r.savePreuploaded(list)
}

// preuploaded lists the hashes put up early, with when.
func (r *Repo) preuploaded() map[string]time.Time {
	out := map[string]time.Time{}
	f, err := os.Open(filepath.Join(r.Dir, preuploadList))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		h, ts, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if n, err := strconv.ParseInt(ts, 10, 64); ok && err == nil {
			out[h] = time.Unix(n, 0)
		}
	}
	return out
}

func (r *Repo) savePreuploaded(list map[string]time.Time) error {
	var lines []string
	for h, t := range list {
		lines = append(lines, fmt.Sprintf("%s %d", h, t.Unix()))
	}
	sort.Strings(lines)
	return store.WriteAtomic(filepath.Join(r.Dir, preuploadList), strings.NewReader(strings.Join(lines, "\n")+"\n"))
}

// keptPreuploads: the hashes put up early that local cleanup keeps listed
// (the rest expire, and the list is rewritten without them).
func (r *Repo) keptPreuploads(now time.Time) map[string]bool {
	list := r.preuploaded()
	keep := map[string]bool{}
	expired := false
	for h, t := range list {
		if now.Sub(t) < preuploadKeep {
			keep[h] = true
		} else {
			delete(list, h)
			expired = true
		}
	}
	if expired {
		r.savePreuploaded(list)
	}
	return keep
}

// pinned: contents a step has here outside the store (a preupload's copy).
var pinMu sync.Mutex

func (r *Repo) pin(h, path string) {
	pinMu.Lock()
	defer pinMu.Unlock()
	if r.pinned == nil {
		r.pinned = map[string]string{}
	}
	r.pinned[h] = path
}

func (r *Repo) unpin(h string) {
	pinMu.Lock()
	defer pinMu.Unlock()
	delete(r.pinned, h)
}

func (r *Repo) pinnedCopy(h string) string {
	pinMu.Lock()
	defer pinMu.Unlock()
	return r.pinned[h]
}

// CleanPreuploads removes copies left by preuploads that stopped (the app
// closed).
func (r *Repo) CleanPreuploads() {
	os.RemoveAll(filepath.Join(r.Dir, preuploadDir))
}
