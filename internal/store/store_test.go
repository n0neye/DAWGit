package store

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPutIsContentAddressedAndIdempotent(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h1, n, err := s.Put(strings.NewReader("kick"))
	if err != nil || n != 4 {
		t.Fatal(err, n)
	}
	h2, _, _ := s.Put(strings.NewReader("kick"))
	if h1 != h2 || !s.Has(h1) {
		t.Fatalf("hash mismatch %s %s", h1, h2)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256([]byte("kick"))); h1 != want {
		t.Fatalf("hash = %s, want %s", h1, want)
	}
	b, err := s.Read(h1)
	if err != nil || string(b) != "kick" {
		t.Fatal(err, string(b))
	}
	if n, _ := os.ReadDir(filepath.Join(s.dir, "tmp")); len(n) != 0 {
		t.Errorf("temp files left behind: %d", len(n))
	}
}

func TestExportAndHashFile(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(filepath.Join(dir, "objects"))
	h, _, _ := s.Put(strings.NewReader("snare"))
	dst := filepath.Join(dir, "out", "a", "snare.wav")
	if err := s.Export(h, dst); err != nil {
		t.Fatal(err)
	}
	got, n, err := HashFile(dst)
	if err != nil || got != h || n != 5 {
		t.Fatalf("%v %s %s %d", err, got, h, n)
	}
	if _, err := s.Open("nope"); err == nil {
		t.Error("expected error for invalid hash")
	}
}
