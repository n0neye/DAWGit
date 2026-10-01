package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A file another program holds for a moment, or marks read-only, is still
// replaced and removed.
func TestReplaceRetries(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "kick.wav")
	os.WriteFile(dst, []byte("old"), 0o644)
	held, err := os.Open(dst) // on Windows: blocks replacing it
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		held.Close()
	}()
	if err := WriteAtomic(dst, strings.NewReader("new")); err != nil {
		t.Fatalf("held file: %v", err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "new" {
		t.Fatalf("content %q", b)
	}
	os.Chmod(dst, 0o444)
	if err := WriteAtomic(dst, strings.NewReader("newer")); err != nil {
		t.Fatalf("read-only file: %v", err)
	}
	os.Chmod(dst, 0o444)
	if err := Remove(dst); err != nil {
		t.Fatalf("remove read-only: %v", err)
	}
	start := time.Now()
	if err := Remove(dst); !os.IsNotExist(err) || time.Since(start) > time.Second {
		t.Fatalf("missing file: %v after %v", err, time.Since(start))
	}
}
