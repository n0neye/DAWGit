// Package applog keeps the desktop app's log in a file: the app has no
// console, so what it logs (and a crash) would otherwise be lost. The file
// lives in the settings folder (logs/dawgit.log), is rotated when it grows,
// and never holds secrets: connection codes, keys and tokens are masked
// before a line is written, so a user can attach it to a bug report.
package applog

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sync"
)

const (
	maxSize = 2 << 20 // a log grows to this, then becomes dawgit.1.log
	keep    = 2       // old logs kept (dawgit.1.log, dawgit.2.log)
)

// Setup sends the standard logger's output (and a crash's) to files in dir,
// as well as to stderr.
func Setup(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	w := &file{path: filepath.Join(dir, "dawgit.log")}
	if err := w.open(); err != nil {
		return err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, w))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	// A crash (a panic the app doesn't catch) goes to crash.log.
	if crash, err := os.OpenFile(filepath.Join(dir, "crash.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		debug.SetCrashOutput(crash, debug.CrashOptions{})
		crash.Close() // SetCrashOutput keeps its own copy
	}
	return nil
}

// file is a log file rotated by size, masking secrets.
type file struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

func (w *file) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fi, _ := f.Stat()
	w.f, w.size = f, 0
	if fi != nil {
		w.size = fi.Size()
	}
	return nil
}

func (w *file) Write(p []byte) (int, error) {
	n := len(p)
	line := Mask(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(line)) > maxSize {
		w.rotate()
	}
	if w.f == nil {
		return n, nil
	}
	m, err := w.f.Write(line)
	w.size += int64(m)
	return n, err
}

// rotate: dawgit.log becomes dawgit.1.log, .1 becomes .2, the oldest goes.
func (w *file) rotate() {
	w.f.Close()
	w.f = nil
	old := func(i int) string { return w.path[:len(w.path)-len(".log")] + "." + string(rune('0'+i)) + ".log" }
	os.Remove(old(keep))
	for i := keep - 1; i >= 1; i-- {
		os.Rename(old(i), old(i+1))
	}
	os.Rename(w.path, old(1))
	w.open()
}

var secrets = []*regexp.Regexp{
	// Connection codes hold storage keys.
	regexp.MustCompile(`dawgit-s3:[A-Za-z0-9+/=_-]+`),
	// key=value / "key": "value" for anything named like a secret.
	regexp.MustCompile(`(?i)((?:secret|token|password|access[_-]?key|authorization|credential)[A-Za-z_]*["']?\s*[:=]\s*["']?)[^"'\s,&}]+`),
	// Signed URLs.
	regexp.MustCompile(`(?i)(X-Amz-(?:Signature|Credential|Security-Token)=)[^&\s"]+`),
}

// Mask hides secrets in a log line.
func Mask(p []byte) []byte {
	out := secrets[0].ReplaceAll(p, []byte("dawgit-s3:…"))
	for _, re := range secrets[1:] {
		out = re.ReplaceAll(out, []byte("${1}…"))
	}
	return out
}
