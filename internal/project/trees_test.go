package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/manifest"
)

// Projects from before format 2 keep their history: new versions are
// format 2, with format-1 parents.
func TestFormat2AfterFormat1History(t *testing.T) {
	root := newProject(t)
	r, err := Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	first := mustSnapshot(t, r, "first")

	// Rewrite history as a format-1 project: the same files, listed in the
	// record.
	old := &Manifest{Version: 1, Parents: []string{}, Author: "yi", Time: first.Time, Message: "old",
		Files: first.Files, External: first.External, Packs: first.Packs, Missing: first.Missing}
	data := old.Seal()
	if err := r.storeSnapshot(old.ID, data); err != nil {
		t.Fatal(err)
	}
	if err := r.setHead(old.ID); err != nil {
		t.Fatal(err)
	}
	assertClean(t, r)

	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("new"), 0o644)
	next := mustSnapshot(t, r, "next")
	if next.Version != 2 || next.Tree == "" || next.Parents[0] != old.ID {
		t.Fatalf("new version: %+v", next)
	}
	// The record lists no files, only how many.
	data, _ = os.ReadFile(r.snapshotPath(next.ID))
	var rec map[string]any
	json.Unmarshal(data, &rec)
	if n, ok := rec["files"].(float64); !ok || int(n) != len(first.Files)+1 {
		t.Fatalf("record files: %v", rec["files"])
	}
	for _, id := range []string{old.ID, next.ID} {
		m, err := r.Load(id)
		if err != nil || len(m.Files) == 0 {
			t.Fatalf("load %s: %v", id[:8], err)
		}
	}
	log, err := r.Log()
	if err != nil || len(log) != 2 {
		t.Fatalf("log: %d %v", len(log), err)
	}

	// Going back and forth works, and cleanup keeps what versions use.
	if _, err := r.GC(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.GoTo(old.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("notes.txt should be gone in the old version")
	}
	if _, _, err := r.GoTo(next.ID, false); err != nil {
		t.Fatal(err)
	}
	assertClean(t, r)
}

// A commit writes trees only for folders that changed.
func TestCommitWritesChangedFoldersOnly(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	for i := range 5 {
		dir := filepath.Join(root, "Stems", string(rune('A'+i)))
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "take.wav"), []byte{byte(i)}, 0o644)
	}
	mustSnapshot(t, r, "first")
	count := func() int {
		n := 0
		filepath.WalkDir(filepath.Join(r.Dir, treesDir), func(_ string, d os.DirEntry, _ error) error {
			if d != nil && !d.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	before := count()
	os.WriteFile(filepath.Join(root, "Stems", "C", "take.wav"), []byte("changed"), 0o644)
	m := mustSnapshot(t, r, "second")
	if added := count() - before; added != 3 { // Stems/C, Stems, the top
		t.Fatalf("%d new trees", added)
	}
	// Trees are read back as they were written.
	var n int
	if err := r.walkTrees([]string{m.Tree}, func(_ string, es []manifest.TreeEntry) error {
		n += len(es)
		return nil
	}); err != nil || n == 0 {
		t.Fatal(err)
	}
}

// The stat cache moves from index.json to index.bin and reads back the same.
func TestIndexBinary(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	mustSnapshot(t, r, "first")
	ix := r.loadIndex()
	if len(ix.entries) == 0 {
		t.Fatal("empty index")
	}
	want := ix.entries
	// An older project: index.json only.
	os.Remove(filepath.Join(r.Dir, indexFile))
	if err := writeJSON(filepath.Join(r.Dir, oldIndexFile), want); err != nil {
		t.Fatal(err)
	}
	ix = r.loadIndex()
	if !ix.dirty || len(ix.entries) != len(want) {
		t.Fatalf("migrated %d of %d", len(ix.entries), len(want))
	}
	if err := ix.save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, oldIndexFile)); !os.IsNotExist(err) {
		t.Fatal("index.json should be gone")
	}
	got := r.loadIndex().entries
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: %+v, want %+v", k, got[k], v)
		}
	}
	// A damaged file: start over (files are hashed again), no error.
	os.WriteFile(filepath.Join(r.Dir, indexFile), []byte("DAWGIT-INDEX-1\n\x05ab"), 0o644)
	if n := len(r.loadIndex().entries); n != 0 {
		t.Fatalf("damaged index gave %d entries", n)
	}
	assertClean(t, r)
}
