package watch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "Samples"), 0o755)
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan []string, 4)
	if err := Dir(ctx, root, 100*time.Millisecond, func(p []string) { got <- p }); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "Samples", "a.wav"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "Samples", "a.wav"), []byte("xy"), 0o644)
	select {
	case p := <-got:
		if !slices.Contains(p, "Samples/a.wav") {
			t.Fatalf("got %v", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no change seen")
	}
	// After cancel the folder is let go: it can be removed.
	cancel()
	time.Sleep(100 * time.Millisecond)
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("still held: %v", err)
	}
}
