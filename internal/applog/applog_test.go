package applog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	for in, leak := range map[string]string{
		"joined with dawgit-s3:eyJ1cmwiOiJzMytodHRwczovL3guci5jb20iLCJrIjoiQUtJRCJ9 ok": "eyJ1cmwi",
		`{"access_key":"AKIDEXAMPLE","secret_key":"wJalrXUt"}`:                          "wJalrXUt",
		"token=3f9c1e2d &x": "3f9c1e2d",
		"GET https://b.r2.dev/x?X-Amz-Signature=abc123def&X-Amz-Credential=AKID%2F": "abc123def",
		"Authorization: AWS4-HMAC-SHA256":                                           "AWS4-HMAC",
	} {
		got := string(Mask([]byte(in)))
		if strings.Contains(got, leak) {
			t.Errorf("%q -> %q still has %q", in, got, leak)
		}
	}
	if got := string(Mask([]byte("backup team1: the backup isn't there"))); got != "backup team1: the backup isn't there" {
		t.Errorf("plain line changed: %q", got)
	}
}

func TestRotate(t *testing.T) {
	dir := t.TempDir()
	w := &file{path: filepath.Join(dir, "dawgit.log")}
	if err := w.open(); err != nil {
		t.Fatal(err)
	}
	line := []byte(strings.Repeat("x", 1000) + "\n")
	for range 3 * maxSize / len(line) {
		w.Write(line)
	}
	w.f.Close()
	for _, name := range []string{"dawgit.log", "dawgit.1.log", "dawgit.2.log"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err != nil || fi.Size() > maxSize {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "dawgit.3.log")); err == nil {
		t.Error("kept too many")
	}
}
