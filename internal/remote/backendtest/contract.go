// Package backendtest is the contract every remote.Backend must satisfy.
// Each implementation runs Run from its own tests.
package backendtest

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"

	"dawgit/internal/manifest"
	"dawgit/internal/remote"
)

// Run checks b against the Backend contract. b must start empty.
func Run(t *testing.T, b remote.Backend) {
	t.Run("info", func(t *testing.T) {
		old, err := b.Info()
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		if err := b.SetInfo(remote.TeamInfo{Name: "Contract Test Team"}); err != nil {
			t.Fatalf("SetInfo: %v", err)
		}
		if got, _ := b.Info(); got.Name != "Contract Test Team" {
			t.Fatalf("Info after SetInfo = %+v", got)
		}
		if old.Name != "" { // leave a live team as it was
			b.SetInfo(old)
		}
	})
	t.Run("objects", func(t *testing.T) { objects(t, b) })
	t.Run("projects", func(t *testing.T) { projects(t, b) })
	t.Run("snapshots", func(t *testing.T) { snapshots(t, b) })
	t.Run("branches", func(t *testing.T) { branches(t, b) })
	t.Run("branch race", func(t *testing.T) { branchRace(t, b) })
	t.Run("workspaces", func(t *testing.T) { workspaces(t, b) })
	t.Run("delete project", func(t *testing.T) { deleteProject(t, b) })
}

func deleteProject(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	v := putVersion(t, b, pid)
	if err := b.UpdateBranch(pid, "main", "", v); err != nil {
		t.Fatal(err)
	}
	if err := b.PutWorkspace(pid, newID(16), map[string]string{"author": "yi"}); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteProject(pid); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	ps, err := b.Projects()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ID == pid {
			t.Fatal("deleted project is still listed")
		}
	}
	if br, err := b.Branches(pid); err == nil && len(br) > 0 {
		t.Fatalf("branches left after delete: %v", br)
	}
}

func newID(n int) string {
	buf := make([]byte, n)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func putBlob(t *testing.T, b remote.Backend, data []byte) string {
	t.Helper()
	h := hashOf(data)
	if err := b.PutObject(h, bytes.NewReader(data)); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	return h
}

// newProject registers a fresh project.
func newProject(t *testing.T, b remote.Backend) string {
	t.Helper()
	pid := newID(16)
	if err := b.PutProject(remote.Project{ID: pid, Name: "Song " + pid[:4]}); err != nil {
		t.Fatalf("PutProject: %v", err)
	}
	return pid
}

// putVersion stores a version (with one file) following the write order.
func putVersion(t *testing.T, b remote.Backend, pid string, parents ...string) string {
	t.Helper()
	blob := putBlob(t, b, []byte("audio "+newID(8)))
	m := &manifest.Manifest{Version: 1, Parents: append([]string{}, parents...), Author: "test",
		Time: "2026-01-01T00:00:00Z", Message: "v " + newID(4),
		Files: []manifest.FileEntry{{Path: "Samples/a.wav", Hash: blob, Size: 1}}}
	data := m.Seal()
	if err := b.PutSnapshot(pid, m.ID, data); err != nil {
		t.Fatalf("PutSnapshot: %v", err)
	}
	return m.ID
}

func objects(t *testing.T, b remote.Backend) {
	data := []byte("kick " + newID(16))
	h := hashOf(data)
	missing, err := b.MissingObjects([]string{h})
	if err != nil || len(missing) != 1 || missing[0] != h {
		t.Fatalf("MissingObjects before put = %v, %v", missing, err)
	}
	putBlob(t, b, data)
	putBlob(t, b, data) // idempotent
	missing, err = b.MissingObjects([]string{h})
	if err != nil || len(missing) != 0 {
		t.Fatalf("MissingObjects after put = %v, %v", missing, err)
	}
	rc, err := b.GetObject(h)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, data) {
		t.Fatalf("GetObject returned %q", got)
	}
	if _, err := b.GetObject(hashOf([]byte("never stored " + newID(8)))); !errors.Is(err, remote.ErrNotFound) {
		t.Fatalf("GetObject of unknown object: %v, want ErrNotFound", err)
	}
	// A large object round-trips (multi-chunk transfer).
	big := bytes.Repeat([]byte(newID(32)), 64*1024) // 4 MiB
	hb := putBlob(t, b, big)
	rc, err = b.GetObject(hb)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, big) {
		t.Fatal("large object corrupted")
	}
}

func projects(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	if err := b.PutProject(remote.Project{ID: pid, Name: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	ps, err := b.Projects()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.ID == pid {
			if p.Name != "Renamed" {
				t.Fatalf("name = %q", p.Name)
			}
			return
		}
	}
	t.Fatalf("project %s not listed in %v", pid, ps)
}

func snapshots(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	v1 := putVersion(t, b, pid)
	v2 := putVersion(t, b, pid, v1)
	unknown := hashOf([]byte("no such version " + newID(8)))
	missing, err := b.MissingSnapshots(pid, []string{v1, v2, unknown})
	if err != nil || len(missing) != 1 || missing[0] != unknown {
		t.Fatalf("MissingSnapshots = %v, %v", missing, err)
	}
	data, err := b.GetSnapshot(pid, v2)
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(v2, data)
	if err != nil || len(m.Parents) != 1 || m.Parents[0] != v1 {
		t.Fatalf("GetSnapshot: %v %+v", err, m)
	}
	if _, err := b.GetSnapshot(pid, unknown); !errors.Is(err, remote.ErrNotFound) {
		t.Fatalf("GetSnapshot of unknown version: %v, want ErrNotFound", err)
	}
}

func wantConflict(t *testing.T, err error, current string) {
	t.Helper()
	var c *remote.ErrConflict
	if !errors.As(err, &c) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if c.Current != current {
		t.Fatalf("ErrConflict.Current = %q, want %q", c.Current, current)
	}
}

func branches(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	heads, err := b.Branches(pid)
	if err != nil || len(heads) != 0 {
		t.Fatalf("Branches of new project = %v, %v", heads, err)
	}
	v1 := putVersion(t, b, pid)
	v2 := putVersion(t, b, pid, v1)

	if err := b.UpdateBranch(pid, "main", "", v1); err != nil {
		t.Fatalf("create: %v", err)
	}
	wantConflict(t, b.UpdateBranch(pid, "main", "", v2), v1) // already exists
	wantConflict(t, b.UpdateBranch(pid, "main", v2, v1), v1) // stale old
	if err := b.UpdateBranch(pid, "main", v1, v2); err != nil {
		t.Fatalf("move: %v", err)
	}
	if err := b.UpdateBranch(pid, "yi-ideas", "", v1); err != nil {
		t.Fatalf("second branch: %v", err)
	}
	heads, err = b.Branches(pid)
	if err != nil || heads["main"] != v2 || heads["yi-ideas"] != v1 || len(heads) != 2 {
		t.Fatalf("Branches = %v, %v", heads, err)
	}
	if err := b.UpdateBranch(pid, "yi-ideas", v1, ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	heads, _ = b.Branches(pid)
	if _, ok := heads["yi-ideas"]; ok || len(heads) != 1 {
		t.Fatalf("after delete: %v", heads)
	}
}

// branchRace: several saves from the same version at once; exactly one wins.
func branchRace(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	base := putVersion(t, b, pid)
	if err := b.UpdateBranch(pid, "main", "", base); err != nil {
		t.Fatal(err)
	}
	const n = 8
	versions := make([]string, n)
	for i := range versions {
		versions[i] = putVersion(t, b, pid, base)
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = b.UpdateBranch(pid, "main", base, versions[i])
		}(i)
	}
	wg.Wait()
	var won []int
	for i, err := range errs {
		var c *remote.ErrConflict
		switch {
		case err == nil:
			won = append(won, i)
		case !errors.As(err, &c):
			t.Fatalf("saver %d: unexpected error %v", i, err)
		}
	}
	if len(won) != 1 {
		t.Fatalf("%d saves won the race, want exactly 1", len(won))
	}
	heads, _ := b.Branches(pid)
	if heads["main"] != versions[won[0]] {
		t.Fatalf("main = %s, want the winner %s", heads["main"], versions[won[0]])
	}
}

type state struct {
	ID    string   `json:"id"`
	Edits []string `json:"edits"`
}

func workspaces(t *testing.T, b remote.Backend) {
	pid := newProject(t, b)
	var none []state
	if err := b.Workspaces(pid, &none); err != nil || len(none) != 0 {
		t.Fatalf("Workspaces of new project = %v, %v", none, err)
	}
	w1, w2 := newID(16), newID(16)
	for _, s := range []state{{w1, []string{"Bass"}}, {w2, []string{"Drums"}}, {w1, []string{"Bass", "Keys"}}} {
		if err := b.PutWorkspace(pid, s.ID, s); err != nil {
			t.Fatal(err)
		}
	}
	var got []state
	if err := b.Workspaces(pid, &got); err != nil {
		t.Fatal(err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	want := []state{{w1, []string{"Bass", "Keys"}}, {w2, []string{"Drums"}}}
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("Workspaces = %v, want %v", got, want)
	}
}
