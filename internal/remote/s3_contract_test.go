package remote_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/backendtest"
	"dawgit/internal/remote/membucket"
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

func TestSetups(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, err := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	a, c := strings.Repeat("a", 32), strings.Repeat("c", 32)
	b.PutSetup(a, []byte(`{"live":[]}`))
	b.PutSetup(c, []byte(`{"packs":[]}`))
	got, err := b.Setups()
	if err != nil || len(got) != 2 || string(got[a]) != `{"live":[]}` {
		t.Fatalf("%v %v", got, err)
	}
	b.DeleteSetup(a)
	if got, _ := b.Setups(); len(got) != 1 {
		t.Fatalf("after delete: %v", got)
	}
	if b.PutSetup("../x", nil) == nil {
		t.Error("bad id")
	}
}

// Every Bucket gets the whole team layer: the contract holds over one kept
// in memory too.
func TestMemoryBucketContract(t *testing.T) {
	mb := membucket.New()
	mb.PageSize = 2
	backendtest.Run(t, remote.NewBucketBackend(mb))
}
