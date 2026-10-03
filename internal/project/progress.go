package project

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Stages reported while saving or downloading a project.
const (
	StageScanning    = "scanning"    // looking for changed files
	StageStoring     = "storing"     // copying changed files into the history
	StageChecking    = "checking"    // asking the team's storage which files it has
	StageUploading   = "uploading"   // sending files to the team
	StageDownloading = "downloading" // fetching files from the team
	StageExporting   = "exporting"   // writing a version to another folder
)

// Progress describes a long-running step: Done of Total items (Total is 0
// when unknown) and, for transfers, Bytes of TotalBytes.
type Progress struct {
	Stage             string
	Done, Total       int
	Bytes, TotalBytes int64
}

// report calls r.OnProgress when set.
func (r *Repo) report(stage string, done, total int) {
	if r.OnProgress != nil {
		r.OnProgress(Progress{Stage: stage, Done: done, Total: total})
	}
}

// transfer tracks the bytes of one upload or download of several files.
// transfer counts an upload or download; files go several at a time, so
// it is safe for concurrent use (and reports one at a time).
type transfer struct {
	r                 *Repo
	stage             string
	mu                sync.Mutex
	done, total       int
	bytes, totalBytes int64
}

func (r *Repo) newTransfer(stage string, files int, totalBytes int64) *transfer {
	return &transfer{r: r, stage: stage, total: files, totalBytes: totalBytes}
}

func (t *transfer) report() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reportLocked()
}

func (t *transfer) reportLocked() {
	if t.r.OnProgress != nil {
		t.r.OnProgress(Progress{Stage: t.stage, Done: t.done, Total: t.total, Bytes: t.bytes, TotalBytes: t.totalBytes})
	}
}

// fileDone counts a finished file.
func (t *transfer) fileDone() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.done++
	t.reportLocked()
}

// shrink takes n bytes off the total: a file goes up compressed.
func (t *transfer) shrink(n int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.totalBytes -= n
	t.reportLocked()
}

// reader counts what passes through rd. Size lets uploads stream a file of
// known size instead of reading it into memory first.
func (t *transfer) reader(rd io.Reader, size int64) *countingReader {
	return &countingReader{rd: rd, size: size, t: t}
}

type countingReader struct {
	rd   io.Reader
	size int64
	t    *transfer
	read int64
}

// undo takes back what was counted: the transfer is tried again.
func (c *countingReader) undo() {
	c.t.mu.Lock()
	c.t.bytes -= c.read
	c.read = 0
	c.t.reportLocked()
	c.t.mu.Unlock()
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.rd.Read(p)
	if n > 0 {
		c.t.mu.Lock()
		c.read += int64(n)
		c.t.bytes += int64(n)
		c.t.reportLocked()
		c.t.mu.Unlock()
	}
	return n, err
}

func (c *countingReader) Size() int64 { return c.size }

// knowSizes remembers the sizes of m's files, so downloads can tell how much
// is left.
func (r *Repo) knowSizes(ms ...*Manifest) {
	if r.sizes == nil {
		r.sizes = map[string]int64{}
	}
	for _, m := range ms {
		if m == nil {
			continue
		}
		for _, f := range m.Files {
			r.sizes[f.Hash] = f.Size
		}
		for _, f := range m.External {
			r.sizes[f.Hash] = f.Size
		}
	}
}

// SetsSignature changes whenever a set in the project root is saved or the
// workspace moves to another version. It only stats files, so it is cheap
// enough to poll every second.
func (r *Repo) SetsSignature() string {
	sets, _ := filepath.Glob(filepath.Join(r.Root, "*.als"))
	sort.Strings(sets)
	var b strings.Builder
	b.WriteString(r.Head())
	for _, s := range sets {
		if fi, err := os.Stat(s); err == nil {
			fmt.Fprintf(&b, "|%s:%d:%d", filepath.Base(s), fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
}
