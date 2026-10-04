package remote

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Reading a team's storage key by key (a backup copies it as it is).

// Item is a key in a team's storage.
type Item struct {
	Key      string // relative to the team's folder: objects/ab/…, projects/…
	Size     int64
	Modified time.Time
}

// List lists every key under prefix ("" for all).
func (b *S3Backend) List(prefix string) ([]Item, error) {
	items, err := b.listAll(prefix)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		out = append(out, Item{Key: it.key, Size: it.size, Modified: it.modified})
	}
	return out, nil
}

// Open reads the bytes stored at key; the caller closes it.
func (b *S3Backend) Open(key string) (io.ReadCloser, error) {
	resp, err := b.do("GET", key, nil, nil, 0, "", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, s3Error(&s3Response{status: resp.StatusCode, body: data})
	}
	return resp.Body, nil
}

// Put writes size bytes from r at key (a backup into another bucket). The
// body goes unsigned; the backup checks sizes afterwards.
func (b *S3Backend) Put(key string, r io.Reader, size int64) error {
	if size > multipartThreshold {
		return b.putMultipart(key, r, size)
	}
	resp, err := b.do("PUT", key, nil, r, size, unsignedPayload, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return s3Error(&s3Response{status: resp.StatusCode, body: data})
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// PutNew writes data at key unless something is there already (restoring
// never overwrites what the team has); created says whether it wrote.
func (b *S3Backend) PutNew(key string, data []byte) (bool, error) {
	r, err := b.put(key, data, http.Header{"If-None-Match": {"*"}})
	if err != nil {
		return false, err
	}
	switch r.status {
	case http.StatusOK:
		return true, nil
	case http.StatusPreconditionFailed, http.StatusConflict:
		return false, nil
	}
	return false, s3Error(r)
}

// BackupStatus is what a member's backup of the team last did, kept in the
// team's storage so everyone knows the team has one (where it goes stays on
// that member's computer).
type BackupStatus struct {
	Kind        string    `json:"kind"` // "folder", "s3"
	LastSuccess time.Time `json:"lastSuccess"`
	LastAttempt time.Time `json:"lastAttempt"`
	Failing     bool      `json:"failing"` // the last attempt failed
}

const backupsDir = "backups/"

// PutBackupStatus records member's backup status.
func (b *S3Backend) PutBackupStatus(memberID string, s BackupStatus) error {
	if !ValidMemberID(memberID) {
		return errors.New("invalid member id")
	}
	data, _ := json.Marshal(s)
	r, err := b.put(backupsDir+memberID+".json", data, nil)
	if err != nil {
		return err
	}
	if r.status != http.StatusOK {
		return s3Error(r)
	}
	return nil
}

// DeleteBackupStatus forgets member's backup (they stopped backing up).
func (b *S3Backend) DeleteBackupStatus(memberID string) error {
	if !ValidMemberID(memberID) {
		return errors.New("invalid member id")
	}
	return b.delete(backupsDir + memberID + ".json")
}

// BackupStatuses maps member id to their backup's status.
func (b *S3Backend) BackupStatuses() (map[string]BackupStatus, error) {
	keys, err := b.list(backupsDir, false)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := map[string]BackupStatus{}
	err = parallel(keys, func(key string) error {
		r, err := b.get(key)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var s BackupStatus
		id := strings.TrimSuffix(strings.TrimPrefix(key, backupsDir), ".json")
		if json.Unmarshal(r.body, &s) == nil && ValidMemberID(id) {
			mu.Lock()
			out[id] = s
			mu.Unlock()
		}
		return nil
	})
	return out, err
}
