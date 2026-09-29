package server

import (
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/manifest"
)

// Data written by DAWGit 0.1 kept all branches in one branches.json.
func TestBranchesJSONIsMigrated(t *testing.T) {
	s, err := OpenStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pid := "0123456789abcdef0123456789abcdef"
	if err := s.PutProject(Project{ID: pid, Name: "Song"}); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{Version: 1, Parents: []string{}, Message: "v1"}
	data := m.Seal()
	if err := s.PutSnapshot(pid, m.ID, data); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(s.projectDir(pid), "branches.json")
	if err := writeJSON(old, map[string]string{"main": m.ID, "yi-ideas": m.ID}); err != nil {
		t.Fatal(err)
	}

	b, err := s.Branches(pid)
	if err != nil || b["main"] != m.ID || b["yi-ideas"] != m.ID || len(b) != 2 {
		t.Fatalf("Branches = %v, %v", b, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("branches.json should be renamed after migration")
	}
	if _, err := os.Stat(filepath.Join(s.branchDir(pid), "main")); err != nil {
		t.Errorf("per-branch file missing: %v", err)
	}
	// Compare-and-swap works on the migrated data.
	if err := s.UpdateBranch(pid, "main", "wrong", m.ID); err == nil {
		t.Error("stale update accepted")
	}
	if err := s.UpdateBranch(pid, "yi-ideas", m.ID, ""); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Branches(pid); len(b) != 1 {
		t.Errorf("after delete: %v", b)
	}
}
