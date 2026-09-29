package remote_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
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

// TestS3BackendContractLive runs the contract against a real service:
//
//	DAWGIT_TEST_STORAGE=<connection code> go test ./internal/remote -run Live -v
//
// Each run uses a fresh folder (prefix) inside the code's bucket and leaves
// its test data there.
func TestS3BackendContractLive(t *testing.T) {
	code := os.Getenv("DAWGIT_TEST_STORAGE")
	if code == "" {
		t.Skip("set DAWGIT_TEST_STORAGE to a connection code to test a real service")
	}
	cfg, err := remote.ParseAddress(code, "")
	if err != nil {
		t.Fatal(err)
	}
	run := make([]byte, 4)
	rand.Read(run)
	cfg.URL = strings.TrimRight(cfg.URL, "/") + "/contract-test-" + hex.EncodeToString(run)
	t.Logf("testing %s", cfg.Display())
	b, err := remote.Open(cfg)
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
