package remote

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"dawgit/internal/manifest"
	"dawgit/internal/remote/s3test"
)

func TestCollectGarbage(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, err := NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	defer func() { gcNow = time.Now }()
	gcNow = func() time.Time { return now }
	fake.Clock = func() time.Time { return now.Add(-30 * 24 * time.Hour) } // a month ago

	put := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		h := hex.EncodeToString(sum[:])
		if err := b.PutObject(h, bytes.NewReader([]byte(s))); err != nil {
			t.Fatal(err)
		}
		return h
	}
	// A format-2 version (files in trees) and a format-1 one.
	inTree, inList := put("kick"), put("snare")
	root, trees, _ := manifest.BuildTrees([]manifest.FileEntry{{Path: "Samples/kick.wav", Hash: inTree, Size: 4}})
	for h, data := range trees {
		if err := b.PutObject(h, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
	}
	b.PutProject(Project{ID: "p1", Name: "Song"})
	v2 := &manifest.Manifest{Version: 2, Parents: []string{}, Tree: root, FileCount: 1, TotalSize: 4}
	if err := b.PutSnapshot("p1", v2.ID, v2.Seal()); err != nil {
		t.Fatal(err)
	}
	v1 := &manifest.Manifest{Version: 1, Parents: []string{}, Files: []manifest.FileEntry{{Path: "snare.wav", Hash: inList, Size: 5}}}
	data := v1.Seal()
	if err := b.PutSnapshot("p1", v1.ID, data); err != nil {
		t.Fatal(err)
	}
	unused, leased := put("deleted project's take"), put("old take a share reuses")
	fake.Clock = nil
	fresh := put("uploaded just now, its version not yet")
	release, err := b.Lease([]string{leased})
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	has := func(h string) bool {
		missing, _ := b.MissingObjects([]string{h})
		return len(missing) == 0
	}
	// First cleanup: unused files are only marked.
	rep, err := b.CollectGarbage(true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 0 || rep.Waiting != 2 || rep.Versions != 2 || !has(unused) {
		t.Fatalf("first cleanup: %+v", rep)
	}
	// A day later: the old unused file goes; the leased and the fresh stay.
	gcNow = func() time.Time { return now.Add(25 * time.Hour) }
	rep, err = b.CollectGarbage(true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Deleted != 1 || has(unused) {
		t.Fatalf("second cleanup: %+v", rep)
	}
	for name, h := range map[string]string{"in a tree": inTree, "in a list": inList, "leased": leased, "fresh": fresh, "tree": root} {
		if !has(h) {
			t.Errorf("%s: deleted", name)
		}
	}
	// Without remove nothing is deleted, even when due.
	gcNow = func() time.Time { return now.Add(9 * 24 * time.Hour) }
	if rep, _ := b.CollectGarbage(false); rep.Deleted != 0 || !has(fresh) {
		t.Fatalf("check only: %+v", rep)
	}
	if rep, _ := b.CollectGarbage(true); rep.Deleted != 1 || has(fresh) {
		t.Fatalf("fresh file, once old and unused: %+v", rep)
	}
}
