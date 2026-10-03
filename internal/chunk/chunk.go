// Package chunk cuts big files into pieces where their content says
// (content-defined chunking: a gear hash, normalized as in FastCDC), so an
// edit changes a few pieces and the rest are stored once. A file kept this
// way is stored as a List of its pieces (see docs/design/chunked-files.md).
//
// The cutting is part of the format: the same bytes must give the same
// pieces in every DAWGit, or nothing matches what is stored already.
package chunk

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

// MinFile: files at least this big are kept as pieces.
const MinFile = 16 << 20

// Piece sizes: never cut before Min, harder to cut before Avg, easier after,
// always at Max.
const (
	Min = 256 << 10
	Avg = 1 << 20
	Max = 4 << 20
)

const (
	maskHard = 1<<22 - 1 // before Avg (2 bits more than log2(Avg))
	maskEasy = 1<<18 - 1 // after Avg (2 bits fewer)
)

var gear [256]uint64

func init() {
	r := rand.New(rand.NewSource(42)) // fixed forever (see package doc)
	for i := range gear {
		gear[i] = r.Uint64()
	}
}

// cut is where the first piece of b ends; b holds Max bytes, or all that is
// left of the file.
func cut(b []byte) int {
	n := len(b)
	if n <= Min {
		return n
	}
	if n > Max {
		n = Max
	}
	var h uint64
	for i := Min; i < n; i++ {
		h = (h << 1) + gear[b[i]]
		m := uint64(maskEasy)
		if i < Avg {
			m = maskHard
		}
		if h&m == 0 {
			return i + 1
		}
	}
	return n
}

// Cut reads r to its end, calling fn with each piece in order. The slice is
// only valid during the call.
func Cut(r io.Reader, fn func(piece []byte) error) error {
	buf := make([]byte, 2*Max)
	start, end := 0, 0
	eof := false
	for {
		if !eof && end-start < Max {
			copy(buf, buf[start:end])
			end -= start
			start = 0
			n, err := io.ReadAtLeast(r, buf[end:], Max-end)
			end += n
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				eof = true
			} else if err != nil {
				return err
			}
		}
		if start == end {
			return nil
		}
		n := cut(buf[start:end])
		if err := fn(buf[start : start+n]); err != nil {
			return err
		}
		start += n
	}
}

// Piece is one piece of a file: the SHA-256 of its bytes, and how many.
type Piece struct {
	Hash string
	Size int64
}

// List is a file as its pieces, in order.
type List struct {
	Pieces []Piece
}

// Size is the file's size.
func (l *List) Size() int64 {
	var n int64
	for _, p := range l.Pieces {
		n += p.Size
	}
	return n
}

// Hashes are the pieces' hashes, each once.
func (l *List) Hashes() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range l.Pieces {
		if !seen[p.Hash] {
			seen[p.Hash] = true
			out = append(out, p.Hash)
		}
	}
	return out
}

const header = "dawgit-chunks 1 fastcdc-gear"

// Encode is the list as text: a header line (format, algorithm, sizes), then
// "<hash> <size>" for each piece.
func (l *List) Encode() []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s %d %d %d\n", header, Min, Avg, Max)
	for _, p := range l.Pieces {
		fmt.Fprintf(&b, "%s %d\n", p.Hash, p.Size)
	}
	return b.Bytes()
}

// Parse reads a list written by Encode (any piece sizes).
func Parse(data []byte) (*List, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	if !sc.Scan() || !strings.HasPrefix(sc.Text(), "dawgit-chunks 1 ") {
		return nil, errors.New("not a chunk list (or one made by a newer DAWGit)")
	}
	l := &List{}
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 || len(f[0]) != 64 {
			return nil, fmt.Errorf("chunk list: bad line %q", sc.Text())
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			return nil, fmt.Errorf("chunk list: bad line %q", sc.Text())
		}
		n, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || n <= 0 || n > 1<<30 {
			return nil, fmt.Errorf("chunk list: bad line %q", sc.Text())
		}
		l.Pieces = append(l.Pieces, Piece{Hash: f[0], Size: n})
	}
	return l, sc.Err()
}

// HashOf is the SHA-256 of b, as hex.
func HashOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// ListFile cuts the file at path into pieces; whole is the SHA-256 of all
// of it.
func ListFile(path string) (l *List, whole string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	h := sha256.New()
	l = &List{}
	err = Cut(f, func(p []byte) error {
		h.Write(p)
		l.Pieces = append(l.Pieces, Piece{Hash: HashOf(p), Size: int64(len(p))})
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return l, hex.EncodeToString(h.Sum(nil)), nil
}
