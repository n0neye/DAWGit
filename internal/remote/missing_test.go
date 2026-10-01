package remote

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"

	"dawgit/internal/remote/s3test"
)

// Asking which objects storage lacks: dense folders are listed (a page at a
// time), the rest asked about one by one; the answer is the same.
func TestMissingObjectsListsDenseFolders(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	fake.PageSize = 5
	b, err := NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	hash := func(data []byte) string {
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	// 40 objects in folder "ab" (several pages), and 30 spread anywhere.
	var dense, spread [][]byte
	for i := 0; len(dense) < 40 || len(spread) < 30; i++ {
		data := []byte(fmt.Sprint("sample ", i))
		switch h := hash(data); {
		case h[:2] == "ab" && len(dense) < 40:
			dense = append(dense, data)
		case h[:2] != "ab" && len(spread) < 30:
			spread = append(spread, data)
		}
	}
	var all, want []string
	for i, data := range append(dense, spread...) {
		h := hash(data)
		all = append(all, h)
		if i%3 == 0 { // a third is stored already
			if err := b.PutObject(h, bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
		} else {
			want = append(want, h)
		}
	}
	fake.Requests["LIST"], fake.Requests["HEAD"] = 0, 0
	got, err := b.MissingObjects(append(all, all[1])) // a repeat, too
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	got = slices.Compact(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("missing:\n got %d %v\nwant %d %v", len(got), got, len(want), want)
	}
	if fake.Requests["LIST"] == 0 || fake.Requests["HEAD"] >= len(all) {
		t.Fatalf("lists %d, heads %d", fake.Requests["LIST"], fake.Requests["HEAD"])
	}
	t.Logf("%d objects: %d lists, %d heads", len(all), fake.Requests["LIST"], fake.Requests["HEAD"])
}
