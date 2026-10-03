package chunk

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func data(seed int64, n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

func pieces(t *testing.T, b []byte) []Piece {
	t.Helper()
	var out []Piece
	var all []byte
	err := Cut(bytes.NewReader(b), func(p []byte) error {
		all = append(all, p...)
		out = append(out, Piece{HashOf(p), int64(len(p))})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(all, b) {
		t.Fatal("pieces don't join into the file")
	}
	return out
}

// The cutting is part of the format: these pieces must never change.
func TestFixedBoundaries(t *testing.T) {
	ps := pieces(t, data(1, 20<<20))
	var sizes []int64
	for _, p := range ps {
		sizes = append(sizes, p.Size)
	}
	want := golden
	if len(sizes) != len(want) {
		t.Fatalf("sizes %v, want %v", sizes, want)
	}
	for i := range want {
		if sizes[i] != want[i] {
			t.Fatalf("sizes %v, want %v", sizes, want)
		}
	}
	if ps[0].Hash != goldenFirst {
		t.Fatalf("first piece %s", ps[0].Hash)
	}
}

func TestSizes(t *testing.T) {
	zeros := make([]byte, 10<<20) // no cut point: Max each
	for i, p := range pieces(t, zeros) {
		if p.Size != Max && i < 2 {
			t.Fatalf("zeros: piece %d is %d", i, p.Size)
		}
	}
	ps := pieces(t, data(2, 64<<20))
	var total int64
	for i, p := range ps {
		if p.Size > Max || (p.Size < Min && i < len(ps)-1) {
			t.Fatalf("piece %d: %d bytes", i, p.Size)
		}
		total += p.Size
	}
	if avg := total / int64(len(ps)); avg < Avg/2 || avg > Avg*2 {
		t.Errorf("average piece %d", avg)
	}
	for _, n := range []int{0, 1, Min, Max, Max + 1} {
		pieces(t, data(3, n))
	}
}

// An edit in the middle changes the pieces around it only.
func TestEditKeepsMostPieces(t *testing.T) {
	old := data(4, 40<<20)
	edited := append(append(append([]byte{}, old[:17<<20]...), []byte("a new actor in the level")...), old[17<<20:]...)
	have := map[string]bool{}
	for _, p := range pieces(t, old) {
		have[p.Hash] = true
	}
	var fresh int64
	for _, p := range pieces(t, edited) {
		if !have[p.Hash] {
			fresh += p.Size
		}
	}
	if fresh > 2*Max {
		t.Errorf("%d bytes new after a small insert", fresh)
	}
}

func TestListFile(t *testing.T) {
	b := data(5, 18<<20)
	p := filepath.Join(t.TempDir(), "level.umap")
	os.WriteFile(p, b, 0o644)
	l, whole, err := ListFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if whole != HashOf(b) || l.Size() != int64(len(b)) {
		t.Fatal("wrong hash or size")
	}
	back, err := Parse(l.Encode())
	if err != nil || len(back.Pieces) != len(l.Pieces) || back.Pieces[3] != l.Pieces[3] {
		t.Fatalf("round trip: %v", err)
	}
	for _, bad := range []string{"", "hello\n", "dawgit-chunks 1 x\nabc 12\n", "dawgit-chunks 2 x\n"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
