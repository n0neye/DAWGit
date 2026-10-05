package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// Reading a team's storage key by key (a backup copies it as it is).

// List lists every key under prefix ("" for all).
func (s *BucketBackend) List(prefix string) ([]Item, error) { return listAll(s.b, prefix) }

// Open reads the bytes stored at key; the caller closes it.
func (s *BucketBackend) Open(key string) (io.ReadCloser, error) { return s.b.Open(key) }

// Put writes size bytes from r at key (a backup into another bucket). The
// body goes unsigned; the backup checks sizes afterwards.
func (s *BucketBackend) Put(key string, r io.Reader, size int64) error {
	return s.b.Put(key, r, size, "", "")
}

// PutNew writes data at key unless something is there already (restoring
// never overwrites what the team has); created says whether it wrote.
func (s *BucketBackend) PutNew(key string, data []byte) (bool, error) {
	err := s.b.Put(key, bytes.NewReader(data), int64(len(data)), "", "*")
	if errors.Is(err, ErrPrecondition) {
		return false, nil
	}
	return err == nil, err
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
func (s *BucketBackend) PutBackupStatus(memberID string, st BackupStatus) error {
	if !ValidMemberID(memberID) {
		return errors.New("invalid member id")
	}
	data, _ := json.Marshal(st)
	return s.put(backupsDir+memberID+".json", data)
}

// DeleteBackupStatus forgets member's backup (they stopped backing up).
func (s *BucketBackend) DeleteBackupStatus(memberID string) error {
	if !ValidMemberID(memberID) {
		return errors.New("invalid member id")
	}
	return s.delete(backupsDir + memberID + ".json")
}

// BackupStatuses maps member id to their backup's status.
func (s *BucketBackend) BackupStatuses() (map[string]BackupStatus, error) {
	out := map[string]BackupStatus{}
	err := s.readAll(backupsDir, func(key string, data []byte) {
		var st BackupStatus
		id := strings.TrimSuffix(strings.TrimPrefix(key, backupsDir), ".json")
		if json.Unmarshal(data, &st) == nil && ValidMemberID(id) {
			out[id] = st
		}
	})
	return out, err
}
