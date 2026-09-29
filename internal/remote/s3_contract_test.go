package remote_test

import (
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/backendtest"
	"dawgit/internal/remote/s3test"
)

func TestS3BackendContract(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	fake.PageSize = 2 // exercise list pagination
	b, err := remote.NewS3(fake.URL, "team", "songs/", "auto", "key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	backendtest.Run(t, b)
}

func TestS3BackendErrors(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	b, _ := remote.NewS3(fake.URL, "missing-bucket", "", "", "key", "secret")
	if _, err := b.Projects(); err == nil || err.Error() != "storage bucket not found" {
		t.Errorf("missing bucket: %v", err)
	}
	if _, err := remote.NewS3("not a url", "team", "", "", "k", "s"); err == nil {
		t.Error("invalid endpoint accepted")
	}
}
