// Package membucket is a remote.Bucket in memory: for tests of what is
// built on Buckets (remote.BucketBackend, backups, the coming brokered
// access), without HTTP or a fake S3.
package membucket

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"dawgit/internal/remote"
)

// Bucket keeps keys in memory. Its zero value is not usable: use New.
type Bucket struct {
	mu   sync.Mutex
	keys map[string]*entry
	next int // etag counter
	// PageSize is how many keys a List page holds (small values exercise
	// paging).
	PageSize int
}

type entry struct {
	data     []byte
	etag     string
	modified time.Time
}

var _ remote.Bucket = (*Bucket)(nil)

// New is an empty bucket.
func New() *Bucket { return &Bucket{keys: map[string]*entry{}, PageSize: 1000} }

func (b *Bucket) Get(key string) ([]byte, string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.keys[key]
	if !ok {
		return nil, "", remote.ErrNotFound
	}
	return bytes.Clone(e.data), e.etag, nil
}

func (b *Bucket) Open(key string) (io.ReadCloser, error) {
	data, _, err := b.Get(key)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (b *Bucket) Exists(key string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.keys[key]
	return ok, nil
}

func (b *Bucket) check(key, cond string) error {
	e, ok := b.keys[key]
	switch {
	case cond == "":
		return nil
	case cond == "*" && ok, cond != "*" && (!ok || e.etag != cond):
		return remote.ErrPrecondition
	}
	return nil
}

func (b *Bucket) Put(key string, r io.Reader, size int64, sum, cond string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if int64(len(data)) != size {
		return fmt.Errorf("membucket: %s: got %d bytes of %d", key, len(data), size)
	}
	if sum != "" {
		if h := sha256.Sum256(data); hex.EncodeToString(h[:]) != sum {
			return fmt.Errorf("membucket: %s: the bytes don't match their SHA-256", key)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.check(key, cond); err != nil {
		return err
	}
	b.next++
	b.keys[key] = &entry{data: data, etag: `"` + strconv.Itoa(b.next) + `"`, modified: time.Now()}
	return nil
}

func (b *Bucket) Delete(key, cond string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.keys[key]; !ok && cond != "" {
		return remote.ErrNotFound
	}
	if err := b.check(key, cond); err != nil {
		return err
	}
	delete(b.keys, key)
	return nil
}

func (b *Bucket) List(prefix, startAfter string, page func([]remote.Item) bool) error {
	b.mu.Lock()
	var items []remote.Item
	for k, e := range b.keys {
		if strings.HasPrefix(k, prefix) && k > startAfter {
			items = append(items, remote.Item{Key: k, Size: int64(len(e.data)), Modified: e.modified})
		}
	}
	b.mu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	for len(items) > 0 {
		n := min(b.PageSize, len(items))
		if !page(items[:n]) {
			return nil
		}
		items = items[n:]
	}
	return nil
}

func (b *Bucket) Folders(dir string) ([]string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	seen := map[string]bool{}
	for k := range b.keys {
		if rest, ok := strings.CutPrefix(k, dir); ok {
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				seen[dir+rest[:i+1]] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out, nil
}
