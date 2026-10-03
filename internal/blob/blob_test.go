package blob

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, name string, content []byte) *Encoded {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	os.WriteFile(p, content, 0o644)
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	e, err := Encode(p, hash, filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(e.Path)
	if s := sha256.Sum256(stored); hex.EncodeToString(s[:]) != e.SHA256 || int64(len(stored)) != e.Size {
		t.Fatalf("%s: wrong size or sha of the stored bytes", name)
	}
	r, err := NewReader(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	back, err := io.ReadAll(r)
	if err != nil || !bytes.Equal(back, content) {
		t.Fatalf("%s: round trip differs (%v)", name, err)
	}
	return e
}

func TestCompressesWhatShrinks(t *testing.T) {
	scene := []byte(strings.Repeat("--- !u!1 &123456\nGameObject:\n  m_Name: Cube\n  m_IsActive: 1\n", 20000))
	e := roundTrip(t, "Main.unity", scene)
	if !e.Encoded || !e.Temp || e.Size > int64(len(scene))/5 {
		t.Errorf("a scene should compress well: %d -> %d", len(scene), e.Size)
	}
	e.Remove()
	if _, err := os.Stat(e.Path); !os.IsNotExist(err) {
		t.Error("temporary file left")
	}
}

func TestKeepsWhatDoesNot(t *testing.T) {
	noise := make([]byte, 3<<20)
	rand.Read(noise)
	for _, name := range []string{"noise.bin", "photo.png"} {
		if e := roundTrip(t, name, noise); e.Encoded || e.Temp {
			t.Errorf("%s: random bytes should go as they are", name)
		}
	}
	if e := roundTrip(t, "tiny.txt", []byte("hi")); e.Encoded {
		t.Error("tiny files go as they are")
	}
}

// A file that starts like a compressed blob is never read as one.
func TestFileStartingWithMagic(t *testing.T) {
	tricky := append([]byte(Magic+"z"), make([]byte, 100)...)
	rand.Read(tricky[len(Magic)+1:])
	if e := roundTrip(t, "tricky.bin", tricky); !e.Encoded {
		t.Error("should be stored behind the header")
	}
	compressible := append([]byte(Magic+"z"), bytes.Repeat([]byte("a"), 10000)...)
	roundTrip(t, "tricky2.txt", compressible)
}

// Blobs from before compression read as they are.
func TestOldBlobs(t *testing.T) {
	for _, old := range [][]byte{nil, []byte("x"), []byte("plain contents of a file")} {
		r, err := NewReader(bytes.NewReader(old))
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := io.ReadAll(r); !bytes.Equal(got, old) {
			t.Errorf("%q read as %q", old, got)
		}
	}
}

func TestBytes(t *testing.T) {
	noise := make([]byte, 5000)
	rand.Read(noise)
	for _, b := range [][]byte{nil, []byte("hi"), noise, bytes.Repeat([]byte("level "), 2000),
		append([]byte(Magic+"z"), noise...)} {
		enc := EncodeBytes(b)
		back, err := DecodeBytes(enc)
		if err != nil || !bytes.Equal(back, b) {
			t.Fatalf("round trip of %d bytes: %v", len(b), err)
		}
		// as a stream too
		r, err := NewReader(bytes.NewReader(enc))
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := io.ReadAll(r); !bytes.Equal(got, b) {
			t.Fatal("stream differs")
		}
	}
	if len(EncodeBytes(bytes.Repeat([]byte("level "), 2000))) > 1000 {
		t.Error("should compress")
	}
}

func TestChunkListBlob(t *testing.T) {
	text := []byte("dawgit-chunks 1 fastcdc-gear 1 2 3\n")
	b := ChunkList(text)
	if !IsChunkList(b[:16]) || IsChunkList([]byte(Magic+"z")) {
		t.Fatal("IsChunkList")
	}
	if _, err := NewReader(bytes.NewReader(b)); err != ErrChunkList {
		t.Fatalf("NewReader: %v", err)
	}
	if _, err := DecodeBytes(b); err != ErrChunkList {
		t.Fatalf("DecodeBytes: %v", err)
	}
	rc, list, err := Open(bytes.NewReader(b))
	if err != nil || !list {
		t.Fatal(err)
	}
	if got, _ := io.ReadAll(rc); !bytes.Equal(got, text) {
		t.Fatalf("%q", got)
	}
}
