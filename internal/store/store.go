// Package store is a content-addressed blob store: objects are named by the
// SHA-256 of their bytes and sharded into <dir>/<first two hex>/<rest>.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Store struct{ dir string }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

// Path returns where an object lives on disk.
func (s *Store) Path(hash string) string {
	return filepath.Join(s.dir, hash[:2], hash[2:])
}

func (s *Store) Has(hash string) bool {
	if !validHash(hash) {
		return false
	}
	_, err := os.Stat(s.Path(hash))
	return err == nil
}

// Put streams r into the store and returns its hash and size. Storing an
// object that already exists is a no-op.
func (s *Store) Put(r io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(filepath.Join(s.dir, "tmp"), "put-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", 0, err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	if s.Has(hash) {
		return hash, n, nil
	}
	dst := s.Path(hash)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", 0, err
	}
	if err := Replace(tmp.Name(), dst); err != nil && !s.Has(hash) {
		return "", 0, err
	}
	return hash, n, nil
}

// PutFile stores the contents of a file.
func (s *Store) PutFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return s.Put(f)
}

// Open returns a reader for an object.
func (s *Store) Open(hash string) (io.ReadCloser, error) {
	if !validHash(hash) {
		return nil, fmt.Errorf("store: invalid hash %q", hash)
	}
	f, err := os.Open(s.Path(hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("store: object %s not found", hash[:12])
	}
	return f, err
}

func (s *Store) Read(hash string) ([]byte, error) {
	r, err := s.Open(hash)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Export writes an object to dst atomically, creating parent directories.
func (s *Store) Export(hash, dst string) error {
	r, err := s.Open(hash)
	if err != nil {
		return err
	}
	defer r.Close()
	return WriteAtomic(dst, r)
}

// WriteAtomic writes r to path via a temp file in the same directory.
func WriteAtomic(path string, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dawgit-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return Replace(tmp.Name(), path)
}

// HashFile returns the SHA-256 of a file without storing it.
func HashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// List returns every object stored, by hash, with its size on disk.
func (s *Store) List() (map[string]int64, error) {
	out := map[string]int64{}
	shards, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue // e.g. tmp
		}
		files, err := os.ReadDir(filepath.Join(s.dir, shard.Name()))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			h := shard.Name() + f.Name()
			if !validHash(h) {
				continue
			}
			if fi, err := f.Info(); err == nil {
				out[h] = fi.Size()
			}
		}
	}
	return out, nil
}
