package project

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"dawgit/internal/store"
)

// scanned is a working file with its size and modification time, as the
// folder listing gives them (no extra call per file).
type scanned struct {
	rel     string
	size    int64
	mtime   int64
	ignored bool // left out by the rules (listed only when asked for)
}

// scanWorkers is how many folders are listed at once: listing folders is
// most of a scan.
const scanWorkers = 8

// scan lists working files as slash-separated paths relative to the root,
// sorted. withIgnored also lists the files the rules leave out (everything
// but DAWGit's own folder).
func (r *Repo) scan(withIgnored bool) ([]scanned, error) {
	rules := r.rules()
	var (
		mu    sync.Mutex
		files []scanned
		first error
		wg    sync.WaitGroup
	)
	sem := make(chan struct{}, scanWorkers)
	fail := func(err error) {
		mu.Lock()
		if first == nil {
			first = err
		}
		mu.Unlock()
	}
	var list func(rel string)
	list = func(rel string) {
		defer wg.Done()
		sem <- struct{}{}
		entries, err := os.ReadDir(r.Abs(rel))
		<-sem
		if err != nil {
			if rel == "" || !errors.Is(err, fs.ErrNotExist) { // a folder removed meanwhile is fine
				fail(err)
			}
			return
		}
		var mine []scanned
		for _, d := range entries {
			p := d.Name()
			if rel != "" {
				p = rel + "/" + p
			}
			if d.IsDir() {
				if p == metaDir || (!withIgnored && rules.SkipDir(p)) {
					continue
				}
				wg.Add(1)
				go list(p)
				continue
			}
			if !d.Type().IsRegular() {
				continue
			}
			ignored := rules.Ignored(p, false)
			if ignored && !withIgnored {
				continue
			}
			fi, err := d.Info()
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					fail(err)
				}
				continue
			}
			mine = append(mine, scanned{p, fi.Size(), fi.ModTime().UnixNano(), ignored})
		}
		mu.Lock()
		files = append(files, mine...)
		mu.Unlock()
	}
	wg.Add(1)
	list("")
	wg.Wait()
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	return files, first
}

// index caches file stat -> content hash, so unchanged samples are not
// rehashed, and so a file rewritten on checkout (sample relinking) still maps
// to the snapshot object it came from.
type indexEntry struct {
	Size  int64  `json:"size"`
	Mtime int64  `json:"mtime"`
	Hash  string `json:"hash"`
	// ObjSize is the size of the object Hash names; it differs from Size
	// when the file was rewritten on checkout (sample relinking).
	ObjSize int64 `json:"obj_size,omitempty"`
}

func (e indexEntry) objSize() int64 {
	if e.ObjSize != 0 {
		return e.ObjSize
	}
	return e.Size
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

// record associates the file's current stat with object hash (of objSize bytes).
func (ix *index) record(abs, rel, hash string, objSize int64) error {
	fi, err := os.Stat(abs)
	if err != nil {
		return err
	}
	ix.entries[rel] = indexEntry{Size: fi.Size(), Mtime: fi.ModTime().UnixNano(), Hash: hash, ObjSize: objSize}
	ix.dirty = true
	return nil
}

// cached returns the content hash of a scanned file when its size and time
// haven't changed since it was hashed.
func (ix *index) cached(f scanned) (string, int64, bool) {
	if e, ok := ix.entries[f.rel]; ok && e.Size == f.size && e.Mtime == f.mtime {
		return e.Hash, e.objSize(), true
	}
	return "", 0, false
}

// hashWorkers hash files in parallel (a first commit hashes every file).
var hashWorkers = min(runtime.NumCPU(), 8)

// workingFiles hashes every tracked working file.
func (r *Repo) workingFiles(ix *index) ([]FileEntry, error) {
	files, err := r.scan(false)
	if err != nil {
		return nil, err
	}
	return r.hashScanned(ix, files)
}

// hashScanned hashes scanned files (those not hashed since they changed).
func (r *Repo) hashScanned(ix *index, files []scanned) ([]FileEntry, error) {
	out := make([]FileEntry, len(files))
	var todo []int
	for i, f := range files {
		out[i].Path = f.rel
		if h, n, ok := ix.cached(f); ok {
			out[i].Hash, out[i].Size = h, n
		} else {
			todo = append(todo, i)
		}
	}
	if len(todo) == 0 {
		return out, nil
	}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		first  error
		next   atomic.Int64
		hashed int
	)
	for range min(hashWorkers, len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				k := int(next.Add(1)) - 1
				if k >= len(todo) {
					return
				}
				i := todo[k]
				abs := r.Abs(files[i].rel)
				// The time before hashing: a file changed meanwhile is hashed
				// again next time.
				fi, err := os.Stat(abs)
				var h string
				var n int64
				if err == nil {
					h, n, err = store.HashFile(abs)
				}
				mu.Lock()
				if err != nil {
					if first == nil {
						first = err
					}
				} else {
					out[i].Hash, out[i].Size = h, n
					ix.entries[files[i].rel] = indexEntry{Size: fi.Size(), Mtime: fi.ModTime().UnixNano(), Hash: h}
					ix.dirty = true
				}
				hashed++
				// Many new files (a first commit): show how reading them goes.
				if len(todo) >= 200 && (hashed%50 == 0 || hashed == len(todo)) {
					r.report(StageScanning, hashed, len(todo))
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out, first
}
