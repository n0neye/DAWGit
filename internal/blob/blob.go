// Package blob is how file contents are kept in a team's storage: as they
// are, or compressed (zstd) when that saves enough. A file is still known by
// the SHA-256 of its own contents; only the bytes in storage differ.
//
// A compressed blob starts with Magic, then a mode byte ('z' zstd, 's' as
// is) and the data. Anything else is the contents as they are (all blobs from
// before compression). A file whose own contents start with Magic is stored
// behind the header ('s'), so it is never taken for a compressed one.
package blob

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Magic starts a blob with a header.
const Magic = "\xffDAWGIT\x01"

const (
	modeZstd   = 'z'
	modeStored = 's'
)

// MinSaving: compressed blobs are kept only when at least this much smaller.
const MinSaving = 0.10

// sampleSize is how much of a file is tried first: a file that doesn't
// compress there is sent as it is without compressing the rest.
const sampleSize = 1 << 20

// incompressible are kinds of files that are compressed already.
var incompressible = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true, ".heic": true, ".avif": true,
	".mp4": true, ".mov": true, ".m4v": true, ".mkv": true, ".webm": true, ".avi": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".flac": true,
	".zip": true, ".7z": true, ".rar": true, ".gz": true, ".xz": true, ".zst": true, ".bz2": true,
	".als": true, // Live's sets are gzip
	".docx": true, ".xlsx": true, ".pptx": true, ".jar": true, ".apk": true, ".unitypackage": true,
}

// Encoded is a file made ready for storage.
type Encoded struct {
	Path    string // the bytes to upload: a temporary file, or the file itself
	Size    int64
	SHA256  string // of the bytes to upload (hex)
	Temp    bool   // Path is temporary: Remove it when done
	Encoded bool   // has the header (compressed or stored behind it)
}

// Remove deletes a temporary encoding.
func (e *Encoded) Remove() {
	if e.Temp {
		os.Remove(e.Path)
	}
}

// Encode prepares the file at path (whose contents hash to hash) for
// storage, compressing it into a temporary file in tmpDir when that saves at
// least MinSaving. Otherwise the file goes as it is (no temporary file),
// unless its contents start with Magic.
func Encode(path, hash, tmpDir string) (*Encoded, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	asIs := &Encoded{Path: path, Size: size, SHA256: hash}
	head, err := readHead(path, len(Magic))
	if err != nil {
		return nil, err
	}
	startsWithMagic := bytes.Equal(head, []byte(Magic))
	if !startsWithMagic && (size < 512 || incompressible[strings.ToLower(filepath.Ext(path))] || !worthTrying(path)) {
		return asIs, nil
	}
	enc, err := write(path, tmpDir, modeZstd)
	if err != nil {
		return nil, err
	}
	if float64(enc.Size) <= float64(size)*(1-MinSaving) {
		return enc, nil
	}
	enc.Remove()
	if !startsWithMagic {
		return asIs, nil
	}
	return write(path, tmpDir, modeStored) // never taken for a compressed blob
}

func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b := make([]byte, n)
	m, err := io.ReadFull(f, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:m], nil
}

// worthTrying compresses the start of the file: does it shrink?
func worthTrying(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sample, err := io.ReadAll(io.LimitReader(f, sampleSize))
	if err != nil || len(sample) == 0 {
		return false
	}
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return false
	}
	defer enc.Close()
	return float64(len(enc.EncodeAll(sample, nil))) <= float64(len(sample))*(1-MinSaving)
}

// write encodes the file with mode into a temporary file.
func write(path, tmpDir string, mode byte) (*Encoded, error) {
	src, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(tmpDir, "blob-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	counter := &countWriter{}
	w := io.MultiWriter(tmp, h, counter)
	if _, err := io.WriteString(w, Magic); err != nil {
		return nil, err
	}
	if _, err := w.Write([]byte{mode}); err != nil {
		return nil, err
	}
	switch mode {
	case modeZstd:
		enc, err := zstd.NewWriter(w, zstd.WithEncoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(enc, src); err != nil {
			enc.Close()
			return nil, err
		}
		if err := enc.Close(); err != nil {
			return nil, err
		}
	default:
		if _, err := io.Copy(w, src); err != nil {
			return nil, err
		}
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	ok = true
	return &Encoded{Path: tmp.Name(), Size: counter.n, SHA256: hex.EncodeToString(h.Sum(nil)), Temp: true, Encoded: true}, nil
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

// NewReader reads a blob from storage as the file's own contents: decoded
// when it has the header, as it is otherwise.
func NewReader(r io.Reader) (io.ReadCloser, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	head, err := br.Peek(len(Magic) + 1)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, err
	}
	if len(head) < len(Magic)+1 || string(head[:len(Magic)]) != Magic {
		return io.NopCloser(br), nil
	}
	mode := head[len(Magic)]
	br.Discard(len(Magic) + 1)
	switch mode {
	case modeStored:
		return io.NopCloser(br), nil
	case modeZstd:
		dec, err := zstd.NewReader(br, zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		return dec.IOReadCloser(), nil
	}
	return nil, fmt.Errorf("blob: unknown mode %q (made by a newer DAWGit?)", mode)
}
