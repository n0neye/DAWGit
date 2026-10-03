package project

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dawgit/internal/blob"
	"dawgit/internal/chunk"
	"dawgit/internal/remote"
	"dawgit/internal/store"
)

// Big files in a team's storage (S3/R2) are kept as pieces: an edit to a
// 300 MB level changes a few of them, and only those go up or down. The
// file's hash names a chunk list; the pieces are objects of their own (see
// docs/design/chunked-files.md). Chunk lists of files on this computer are
// kept in .dawgit/chunks, so a download can take the pieces it already has
// from them.

const chunksDir = "chunks"

// pieceTransfers is how many pieces of one file go up or down at once.
const pieceTransfers = 6

// chunkStore is c when file of size bytes are kept there as pieces.
func chunkStore(c remote.Backend, size int64) (remote.BodyStore, bool) {
	bs, ok := c.(remote.BodyStore)
	return bs, ok && size >= chunk.MinFile
}

func (r *Repo) chunkListPath(h string) string { return filepath.Join(r.Dir, chunksDir, h) }

func (r *Repo) saveChunkList(h string, l *chunk.List) {
	if os.MkdirAll(filepath.Join(r.Dir, chunksDir), 0o755) == nil {
		store.WriteAtomic(r.chunkListPath(h), bytes.NewReader(l.Encode()))
	}
}

func (r *Repo) loadChunkList(h string) *chunk.List {
	data, err := os.ReadFile(r.chunkListPath(h))
	if err != nil {
		return nil
	}
	l, err := chunk.Parse(data)
	if err != nil {
		return nil
	}
	return l
}

// tidyChunkLists forgets the chunk lists of files no longer on this computer.
func (r *Repo) tidyChunkLists() {
	entries, _ := os.ReadDir(filepath.Join(r.Dir, chunksDir))
	for _, e := range entries {
		if r.localCopy(e.Name()) == "" {
			os.Remove(filepath.Join(r.Dir, chunksDir, e.Name()))
		}
	}
}

// uploadChunked puts the file h (at src, size bytes) as pieces: the pieces
// storage lacks, then the chunk list. hold leases the pieces (cleanup must
// not delete one this share relies on).
func (r *Repo) uploadChunked(bs remote.BodyStore, c remote.Backend, h, src string, size int64, t *transfer,
	hold func([]string) error) error {
	l, whole, err := chunk.ListFile(src)
	if err != nil {
		return err
	}
	changed := fmt.Errorf("%s changed while it was being uploaded: save again", filepath.Base(src))
	if whole != h {
		return changed
	}
	r.saveChunkList(h, l)
	hashes := l.Hashes()
	if err := hold(hashes); err != nil {
		return err
	}
	missing, err := c.MissingObjects(hashes)
	if err != nil {
		return err
	}
	need := map[string]bool{}
	for _, p := range missing {
		need[p] = true
	}
	var needBytes int64
	counted := map[string]bool{}
	for _, p := range l.Pieces {
		if need[p.Hash] && !counted[p.Hash] {
			counted[p.Hash] = true
			needBytes += p.Size
		}
	}
	if size < 0 {
		size = l.Size()
		t.shrink(-size)
	}
	t.shrink(size - needBytes)

	// The pieces storage lacks, a few at a time.
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		sem      = make(chan struct{}, pieceTransfers)
	)
	failed := func() error {
		mu.Lock()
		defer mu.Unlock()
		return firstErr
	}
	i := 0
	sent := map[string]bool{}
	err = chunk.Cut(f, func(p []byte) error {
		if i >= len(l.Pieces) || chunk.HashOf(p) != l.Pieces[i].Hash {
			return changed
		}
		ph := l.Pieces[i].Hash
		i++
		if !need[ph] || sent[ph] {
			return nil
		}
		sent[ph] = true
		if err := failed(); err != nil {
			return err
		}
		raw := len(p)
		body := blob.EncodeBytes(append([]byte(nil), p...))
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			t.shrink(int64(raw - len(body)))
			bodySHA := chunk.HashOf(body)
			err := remote.Retry(remote.RetryAttempts, func() error {
				cr := t.reader(bytes.NewReader(body), int64(len(body)))
				if err := bs.PutObjectBody(ph, cr, int64(len(body)), bodySHA); err != nil {
					cr.undo()
					return err
				}
				return nil
			})
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("piece %s: %w", short(ph), err)
				}
				mu.Unlock()
			}
		}()
		return nil
	})
	wg.Wait()
	if err == nil {
		err = failed()
	}
	if err == nil && i != len(l.Pieces) {
		err = changed
	}
	if err != nil {
		return err
	}

	// Then the list: never there without its pieces.
	if err := remote.Retry(remote.RetryAttempts, func() error { return bs.MarkChunked(h) }); err != nil {
		return err
	}
	body := blob.ChunkList(l.Encode())
	return remote.Retry(remote.RetryAttempts, func() error {
		return bs.PutObjectBody(h, bytes.NewReader(body), int64(len(body)), chunk.HashOf(body))
	})
}

// pieceAt is where a piece is in a file on this computer.
type pieceAt struct {
	path      string
	off, size int64
}

// localPieces finds the pieces of the big files on this computer (those
// with a chunk list here).
func (r *Repo) localPieces() map[string]pieceAt {
	at := map[string]pieceAt{}
	entries, _ := os.ReadDir(filepath.Join(r.Dir, chunksDir))
	for _, e := range entries {
		h := e.Name()
		if strings.HasPrefix(h, ".") {
			continue
		}
		p := r.localCopy(h)
		if p == "" {
			continue
		}
		l := r.loadChunkList(h)
		if l == nil {
			continue
		}
		var off int64
		for _, pc := range l.Pieces {
			if _, ok := at[pc.Hash]; !ok {
				at[pc.Hash] = pieceAt{p, off, pc.Size}
			}
			off += pc.Size
		}
	}
	return at
}

// downloadChunked gets the file h kept as the pieces l into the store:
// pieces found on this computer are taken from here, the rest downloaded.
func (r *Repo) downloadChunked(c remote.Backend, h string, l *chunk.List, t *transfer,
	local func() map[string]pieceAt) error {
	here := local()
	tmpRoot := filepath.Join(r.Dir, "objects", "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(tmpRoot, "pieces-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	counted := map[string]bool{} // downloaded pieces count when they arrive
	var fetch []string
	for _, ph := range l.Hashes() {
		if _, ok := here[ph]; !ok {
			fetch = append(fetch, ph)
			counted[ph] = true
		}
	}
	if err := inParallelN(pieceTransfers, fetch, func(ph string) error {
		return r.fetchPiece(c, ph, filepath.Join(tmp, ph), t)
	}); err != nil {
		return err
	}

	// Join them, in order, into the store (which checks the whole file).
	pr, pw := io.Pipe()
	go func() { pw.CloseWithError(r.joinPieces(c, l, here, tmp, counted, pw, t)) }()
	got, _, err := r.Store.Put(pr)
	pr.CloseWithError(io.ErrClosedPipe)
	if err != nil {
		return err
	}
	if got != h {
		return fmt.Errorf("content hash mismatch")
	}
	r.saveChunkList(h, l)
	return nil
}

func (r *Repo) joinPieces(c remote.Backend, l *chunk.List, here map[string]pieceAt, tmp string,
	counted map[string]bool, w io.Writer, t *transfer) error {
	files := map[string]*os.File{}
	defer func() {
		for _, f := range files {
			f.Close()
		}
	}()
	for _, p := range l.Pieces {
		var data []byte
		if at, ok := here[p.Hash]; ok {
			f := files[at.path]
			if f == nil {
				f, _ = os.Open(at.path)
				files[at.path] = f
			}
			if f != nil {
				buf := make([]byte, at.size)
				if _, err := f.ReadAt(buf, at.off); err == nil && chunk.HashOf(buf) == p.Hash {
					data = buf
				}
			}
		}
		if data == nil {
			path := filepath.Join(tmp, p.Hash)
			b, err := os.ReadFile(path)
			if err != nil { // the file here changed meanwhile: download the piece after all
				if !counted[p.Hash] {
					counted[p.Hash] = true
					if err := r.fetchPiece(c, p.Hash, path, t); err != nil {
						return err
					}
				}
				if b, err = os.ReadFile(path); err != nil {
					return err
				}
			}
			data = b
		}
		if !counted[p.Hash] {
			t.count(p.Size)
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

// fetchPiece downloads the piece ph, checks it and writes it to dst.
func (r *Repo) fetchPiece(c remote.Backend, ph, dst string, t *transfer) error {
	var data []byte
	err := remote.Retry(remote.RetryAttempts, func() error {
		raw, err := c.GetObject(ph)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(raw)
		raw.Close()
		if err != nil {
			return err
		}
		if data, err = blob.DecodeBytes(body); err != nil {
			return err
		}
		if chunk.HashOf(data) != ph {
			return fmt.Errorf("piece %s: content hash mismatch", short(ph))
		}
		return nil
	})
	if err != nil {
		return err
	}
	t.count(int64(len(data)))
	return os.WriteFile(dst, data, 0o644)
}
