package backup

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dawgit/internal/remote"
)

// Dest is where a backup goes: a folder (Folder) or a bucket (Bucket).
type Dest interface {
	// Kind: "folder" or "s3"; Name: where, for people (no keys).
	Kind() string
	Name() string
	// Begin readies a run; End tidies after it.
	Begin() error
	End()
	// Stat: the backup's copy of key, if it has one: its size and when it
	// was written (for a folder, the team's own time of the key).
	Stat(key string) (size int64, modified time.Time, ok bool)
	// Put copies key; a copy that fails leaves no half key in place.
	Put(key string, r io.Reader, it remote.Item) error
	// Read and Write a small key (Read: fs.ErrNotExist when absent).
	Read(key string) ([]byte, error)
	Write(key string, data []byte) error
	// Empty: nothing in it yet (a folder may hold what Windows puts in any).
	Empty() (bool, error)
	// Size: bytes the backup holds.
	Size() int64
}

// --- a folder ---

type folder struct{ dir string }

// Folder is a backup in the folder dir.
func Folder(dir string) Dest { return &folder{dir: dir} }

func (f *folder) Kind() string { return "folder" }
func (f *folder) Name() string { return f.dir }

func (f *folder) path(key string) string { return filepath.Join(f.dir, filepath.FromSlash(key)) }

func (f *folder) Begin() error { return os.MkdirAll(filepath.Join(f.dir, ".tmp"), 0o755) }
func (f *folder) End()         { os.RemoveAll(filepath.Join(f.dir, ".tmp")) }

func (f *folder) Stat(key string) (int64, time.Time, bool) {
	fi, err := os.Stat(f.path(key))
	if err != nil {
		return 0, time.Time{}, false
	}
	return fi.Size(), fi.ModTime(), true
}

// Put goes through a temporary file, renamed into place once complete, and
// keeps the key's time.
func (f *folder) Put(key string, r io.Reader, it remote.Item) error {
	tmp, err := os.CreateTemp(filepath.Join(f.dir, ".tmp"), "copy-*")
	if err != nil {
		return err
	}
	_, err = io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	path := f.path(key)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	os.Chtimes(path, it.Modified, it.Modified)
	return nil
}

func (f *folder) Read(key string) ([]byte, error) { return os.ReadFile(f.path(key)) }

func (f *folder) Write(key string, data []byte) error {
	path := f.path(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (f *folder) Empty() (bool, error) {
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return false, err
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		// What Windows and macOS put in any folder.
		if n := strings.ToLower(e.Name()); n != "desktop.ini" && n != ".ds_store" && n != "thumbs.db" {
			return false, nil
		}
	}
	return true, nil
}

func (f *folder) Size() int64 {
	var n int64
	filepath.WalkDir(f.dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// --- a bucket ---

type bucket struct {
	s3   *remote.S3Backend
	name string

	mu  sync.Mutex
	has map[string]remote.Item // listed at Begin, then kept up to date
}

// Bucket is a backup in S3-compatible storage (another bucket, or another
// folder of one).
func Bucket(cfg remote.Config) (Dest, error) {
	b, err := remote.Open(cfg)
	if err != nil {
		return nil, err
	}
	s3, ok := b.(*remote.S3Backend)
	if !ok {
		return nil, errors.New("a backup goes to a folder or to S3-compatible storage")
	}
	return &bucket{s3: s3, name: StorageName(cfg)}, nil
}

// StorageName shows where cfg points (endpoint host, bucket, folder; no keys).
func StorageName(cfg remote.Config) string {
	s, ok := remote.StorageOf(cfg)
	if !ok {
		return ""
	}
	host := strings.TrimPrefix(strings.TrimPrefix(s.Endpoint, "https://"), "http://")
	return host + "/" + s.Bucket + "/" + s.Folder
}

func (b *bucket) Kind() string { return "s3" }
func (b *bucket) Name() string { return b.name }

// Begin lists the backup once: far cheaper than asking key by key.
func (b *bucket) Begin() error {
	items, err := b.s3.List("")
	if err != nil {
		return err
	}
	has := make(map[string]remote.Item, len(items))
	for _, it := range items {
		has[it.Key] = it
	}
	b.mu.Lock()
	b.has = has
	b.mu.Unlock()
	return nil
}

func (b *bucket) End() {}

func (b *bucket) Stat(key string) (int64, time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	it, ok := b.has[key]
	return it.Size, it.Modified, ok
}

func (b *bucket) Put(key string, r io.Reader, it remote.Item) error {
	if err := b.s3.Put(key, r, it.Size); err != nil {
		return err
	}
	b.mu.Lock()
	if b.has != nil {
		b.has[key] = remote.Item{Key: key, Size: it.Size, Modified: time.Now()}
	}
	b.mu.Unlock()
	return nil
}

func (b *bucket) Read(key string) ([]byte, error) {
	r, err := b.s3.Open(key)
	if errors.Is(err, remote.ErrNotFound) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (b *bucket) Write(key string, data []byte) error {
	return b.s3.Put(key, bytes.NewReader(data), int64(len(data)))
}

func (b *bucket) Empty() (bool, error) {
	items, err := b.s3.List("")
	if err != nil {
		return false, err
	}
	for _, it := range items {
		if !strings.HasPrefix(it.Key, "check/") { // CheckBackup's scratch, if left
			return false, nil
		}
	}
	return true, nil
}

func (b *bucket) Size() int64 {
	items, err := b.s3.List("")
	if err != nil {
		return 0
	}
	var n int64
	for _, it := range items {
		n += it.Size
	}
	return n
}
