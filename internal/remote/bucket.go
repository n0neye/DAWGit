package remote

import (
	"errors"
	"io"
	"time"
)

// Bucket is the key storage a team's data lives in: an S3-compatible bucket
// (s3Bucket), and later the hosted service's brokered access. It knows
// nothing about DAWGit: keys, bytes and a few conditions. Everything a
// team stores (projects, versions, branches, members, backups' records,
// cleanup's leases…) is built once on top of it (BucketBackend), so every
// kind of Bucket gets every feature.
//
// Keys are relative to the team's folder ("objects/ab/…", "team.json").
type Bucket interface {
	// Get reads a key whole, with its etag (for a conditional Put or
	// Delete). ErrNotFound when absent.
	Get(key string) (data []byte, etag string, err error)
	// Open streams a key; the caller closes it. ErrNotFound when absent.
	Open(key string) (io.ReadCloser, error)
	// Exists says whether key is there.
	Exists(key string) (bool, error)
	// Put writes size bytes from r at key. sum is the body's hex SHA-256
	// when the caller knows it (storage checks the upload against it), ""
	// when not. cond: "" writes in any case, "*" only if key is absent,
	// anything else only if key still has that etag; ErrPrecondition when
	// the condition doesn't hold.
	Put(key string, r io.Reader, size int64, sum, cond string) error
	// Delete removes key (absent is fine); cond as in Put ("" or an etag).
	Delete(key, cond string) error
	// List calls page with the keys under prefix, in key order, a page at
	// a time, from after startAfter ("" for the start); page returning
	// false stops early.
	List(prefix, startAfter string, page func([]Item) bool) error
	// Folders lists the "folders" right under dir ("projects/" →
	// "projects/<id>/").
	Folders(dir string) ([]string, error)
}

// Item is a key in a bucket.
type Item struct {
	Key      string // relative to the team's folder: objects/ab/…, projects/…
	Size     int64
	Modified time.Time
}

// ErrPrecondition: a conditional write's condition didn't hold (the key
// exists, or changed since it was read).
var ErrPrecondition = errors.New("storage: the key changed meanwhile")

// listAll returns every key under prefix with its size and time.
func listAll(b Bucket, prefix string) ([]Item, error) {
	var out []Item
	err := b.List(prefix, "", func(page []Item) bool {
		out = append(out, page...)
		return true
	})
	return out, err
}

// listKeys returns the keys under prefix.
func listKeys(b Bucket, prefix string) ([]string, error) {
	items, err := listAll(b, prefix)
	if err != nil {
		return nil, err
	}
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	return keys, nil
}
