package remote

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"

	"dawgit/internal/remote/s3test"
)

// Big objects go up in parts; a failing part aborts the upload.
func TestMultipartUpload(t *testing.T) {
	defer func(th, ps int64) { multipartThreshold, partSize = th, ps }(multipartThreshold, partSize)
	multipartThreshold, partSize = 100<<10, 40<<10 // 100 KB, 40 KB parts

	fake := s3test.New("band")
	defer fake.Close()
	b, err := NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 250<<10) // 7 parts, the last one short
	rand.Read(data)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	if err := b.PutObject(hash, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if fake.Requests["MULTIPART"] != 1 {
		t.Fatalf("multipart uploads: %d", fake.Requests["MULTIPART"])
	}
	rc, err := b.GetObject(hash)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("stored %d bytes, want %d", len(got), len(data))
	}

	// Small objects still go in one request.
	small := []byte("small")
	ss := sha256.Sum256(small)
	if err := b.PutObject(hex.EncodeToString(ss[:]), bytes.NewReader(small)); err != nil || fake.Requests["MULTIPART"] != 1 {
		t.Fatalf("small object: %v, multipart %d", err, fake.Requests["MULTIPART"])
	}

	// A part fails: the upload is aborted, nothing stored.
	fake.FailPart = 3
	other := append([]byte("x"), data...)
	os := sha256.Sum256(other)
	if err := b.PutObject(hex.EncodeToString(os[:]), bytes.NewReader(other)); err == nil {
		t.Fatal("a failing part went unnoticed")
	}
	if fake.Uploads() != 0 {
		t.Errorf("%d multipart uploads left behind", fake.Uploads())
	}
	if missing, _ := b.MissingObjects([]string{hex.EncodeToString(os[:])}); len(missing) != 1 {
		t.Error("a failed upload left an object")
	}
}
