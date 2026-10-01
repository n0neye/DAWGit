package project

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// localObjects splits the objects stored here into sets and other files.
func localObjects(t *testing.T, r *Repo) (sets, others int) {
	t.Helper()
	all, setHashes, err := r.referenced()
	if err != nil {
		t.Fatal(err)
	}
	for h := range all {
		if !r.Store.Has(h) {
			continue
		}
		if setHashes[h] {
			sets++
		} else {
			others++
		}
	}
	return sets, others
}

func firstSample(t *testing.T, r *Repo) string {
	t.Helper()
	m, _ := r.Load(r.Head())
	for _, f := range m.Files {
		if strings.HasPrefix(f.Path, "Samples/") && !isSet(f.Path) {
			return f.Path
		}
	}
	t.Fatal("no sample in the project")
	return ""
}

func TestTeamFilesLiveInTheTeamStorage(t *testing.T) {
	a, b := team(t)
	freed, err := a.PruneObjects()
	if err != nil || freed == 0 {
		t.Fatalf("prune: freed %d, %v", freed, err)
	}
	if sets, others := localObjects(t, a); sets == 0 || others != 0 {
		t.Fatalf("after prune: %d sets, %d other files kept", sets, others)
	}
	assertClean(t, a)

	// A new sample is committed; unchanged ones are not copied in again.
	sample := firstSample(t, a)
	original, _ := os.ReadFile(a.Abs(sample))
	os.WriteFile(a.Abs("Samples/new.wav"), []byte("RIFF-new"), 0o644)
	if _, res, err := a.Save("new sample", Strategy("fail")); err != nil || res.Action != "published" {
		t.Fatalf("save: %v %+v", err, res)
	}
	if _, others := localObjects(t, a); others != 1 {
		t.Fatalf("commit copied %d files into the store, want only the new one", others)
	}
	a.PruneObjects()
	if _, others := localObjects(t, a); others != 0 {
		t.Fatalf("new sample kept after it was uploaded")
	}

	// Restoring a changed sample downloads it again.
	os.WriteFile(a.Abs(sample), []byte("scribbled over"), 0o644)
	if err := a.RestoreFile(sample, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(a.Abs(sample)); !bytes.Equal(got, original) {
		t.Fatal("restored sample differs")
	}

	// Going back to a version with a sample deleted since brings it back.
	first := a.Head()
	os.Remove(a.Abs("Samples/new.wav"))
	if _, _, err := a.Save("drop new sample", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	a.PruneObjects()
	if _, _, err := a.GoTo(first, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(a.Abs("Samples/new.wav")); string(got) != "RIFF-new" {
		t.Fatalf("sample of the older version: %q", got)
	}
	if _, _, err := a.GoTo("latest", false); err != nil {
		t.Fatal(err)
	}

	// The other computer takes the new versions in and prunes too.
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	assertClean(t, b)
	if _, err := b.PruneObjects(); err != nil {
		t.Fatal(err)
	}

	// Garbage: an object no version refers to.
	h, _, _ := a.Store.Put(strings.NewReader("left over"))
	if freed, err := a.GC(); err != nil || freed == 0 || a.Store.Has(h) {
		t.Fatalf("gc: freed %d, %v, still there: %v", freed, err, a.Store.Has(h))
	}
	assertClean(t, a)
}

func TestLeavingTheTeam(t *testing.T) {
	a, b := team(t)
	first := a.Head()
	sample := firstSample(t, a)
	os.Remove(a.Abs(sample))
	if _, _, err := a.Save("drop a sample", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	a.PruneObjects()

	// Keeping only what is here: the current version works, the older one
	// needs the team.
	if err := a.PrepareDetach(false); err != nil {
		t.Fatal(err)
	}
	a.Config.Remote = nil
	a.SaveConfig()
	ms, _ := a.allManifests()
	missing := a.MissingHere(ms)
	if !missing[first] || missing[a.Head()] {
		t.Fatalf("missing here: %v (first %s, head %s)", missing, first[:8], a.Head()[:8])
	}
	if _, _, err := a.GoTo(first, false); !errors.Is(err, ErrNotHere) {
		t.Fatalf("going to an incomplete version: %v", err)
	}
	if _, err := a.Snapshot("still works"); err != nil && !errors.Is(err, ErrNothingToSnapshot) {
		t.Fatalf("commit after leaving: %v", err)
	}

	// Downloading the whole history first: everything works without the team.
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b.PruneObjects()
	if n, size, _ := b.HistoryNotHere(); n == 0 || size == 0 {
		t.Fatalf("nothing to download? %d files, %d bytes", n, size)
	}
	if err := b.PrepareDetach(true); err != nil {
		t.Fatal(err)
	}
	b.Config.Remote = nil
	b.SaveConfig()
	if n, _, _ := b.HistoryNotHere(); n != 0 {
		t.Fatalf("%d files still not here", n)
	}
	if _, err := os.Stat(filepath.Join(b.Dir, remoteObjectsFile)); !errors.Is(err, os.ErrNotExist) {
		t.Error("remote-objects list kept after downloading everything")
	}
	if _, _, err := b.GoTo(first, false); err != nil {
		t.Fatalf("older version without the team: %v", err)
	}
	if _, err := os.Stat(b.Abs(sample)); err != nil {
		t.Errorf("sample of the older version: %v", err)
	}
}
