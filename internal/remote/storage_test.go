package remote

import (
	"strings"
	"testing"

	"dawgit/internal/remote/s3test"
)

func TestStorageConfig(t *testing.T) {
	acct := "0123456789abcdef0123456789abcdef"
	for _, c := range []struct {
		in   Storage
		url  string
		fail string
	}{
		{in: Storage{Endpoint: acct, Bucket: "band"}, url: "s3+https://" + acct + ".r2.cloudflarestorage.com/band/dawgit"},
		{in: Storage{Endpoint: "https://" + acct + ".r2.cloudflarestorage.com/band"}, url: "s3+https://" + acct + ".r2.cloudflarestorage.com/band/dawgit"},
		{in: Storage{Endpoint: "s3.example.com", Bucket: "b", Folder: "/songs/"}, url: "s3+https://s3.example.com/b/songs"},
		{in: Storage{Endpoint: "https://x.example.com/other", Bucket: "b"}, fail: "ends in /other"},
		{in: Storage{Endpoint: "https://x.example.com"}, fail: "bucket"},
		{in: Storage{Endpoint: "", Bucket: "b"}, fail: "endpoint"},
	} {
		if c.fail == "" {
			c.in.AccessKey, c.in.SecretKey = "k", "s"
		}
		cfg, err := c.in.Config()
		if c.fail != "" {
			if err == nil || !strings.Contains(err.Error(), c.fail) {
				t.Errorf("%+v: want error with %q, got %v", c.in, c.fail, err)
			}
			continue
		}
		if err != nil || cfg.URL != c.url || cfg.Region != "auto" {
			t.Errorf("%+v: got %+v %v", c.in, cfg, err)
			continue
		}
		back, ok := StorageOf(cfg)
		if again, _ := back.Config(); !ok || again != cfg {
			t.Errorf("round trip: %+v -> %+v", cfg, back)
		}
	}
	if _, err := (Storage{Endpoint: acct, Bucket: "b"}).Config(); err == nil {
		t.Error("missing keys accepted")
	}
}

func TestCheck(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	cfg, err := Storage{Endpoint: fake.URL, Bucket: "band", AccessKey: "k", SecretKey: "s"}.Config()
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(cfg); err != nil {
		t.Fatalf("check: %v", err)
	}
	b, _ := Open(cfg)
	if keys, _ := b.(*S3Backend).list("check/", false); len(keys) != 0 {
		t.Errorf("test file left behind: %v", keys)
	}

	wrong := cfg
	wrong.URL = strings.Replace(cfg.URL, "/band/", "/nope/", 1)
	if err := Check(wrong); err == nil || !strings.Contains(err.Error(), "no bucket") {
		t.Errorf("missing bucket: %v", err)
	}
	fake.IgnoreConditions = true
	if err := Check(cfg); err == nil || !strings.Contains(err.Error(), "conditional") {
		t.Errorf("no conditional writes: %v", err)
	}
}
