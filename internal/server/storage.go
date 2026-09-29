// Package server is the self-hosted DAWGit server: a blob store shared by all
// projects, plus per-project snapshots and branches, kept as plain files so a
// backup is a folder copy.
//
//	<data>/objects/ab/cdef...              blobs (sha256)
//	<data>/projects/<id>/project.json       {id, name}
//	<data>/projects/<id>/snapshots/<id>.json
//	<data>/projects/<id>/branches/<name>    version id (one file per branch)
//	<data>/projects/<id>/workspaces/<id>.json
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"dawgit/internal/manifest"
	"dawgit/internal/store"
)

var (
	idRE     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	projRE   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	branchRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ErrConflict is returned when a branch moved since the client last saw it.
type ErrConflict struct{ Current string }

func (e *ErrConflict) Error() string { return "branch was updated by someone else" }

type Storage struct {
	dir     string
	Objects *store.Store
	mu      sync.Mutex // guards branch files and project.json
}

func OpenStorage(dir string) (*Storage, error) {
	objs, err := store.Open(filepath.Join(dir, "objects"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
		return nil, err
	}
	return &Storage{dir: dir, Objects: objs}, nil
}

func (s *Storage) projectDir(pid string) string { return filepath.Join(s.dir, "projects", pid) }

func (s *Storage) Projects() ([]Project, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "projects"))
	if err != nil {
		return nil, err
	}
	var out []Project
	for _, e := range entries {
		var p Project
		if readJSON(filepath.Join(s.projectDir(e.Name()), "project.json"), &p) == nil {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// PutProject creates a project or renames it.
func (s *Storage) PutProject(p Project) error {
	if !projRE.MatchString(p.ID) {
		return fmt.Errorf("invalid project id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Join(s.projectDir(p.ID), "snapshots"), 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.projectDir(p.ID), "project.json"), p)
}

func (s *Storage) HasProject(pid string) bool {
	if !projRE.MatchString(pid) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.projectDir(pid), "project.json"))
	return err == nil
}

func (s *Storage) snapshotPath(pid, id string) string {
	return filepath.Join(s.projectDir(pid), "snapshots", id+".json")
}

func (s *Storage) HasSnapshot(pid, id string) bool {
	if !idRE.MatchString(id) {
		return false
	}
	_, err := os.Stat(s.snapshotPath(pid, id))
	return err == nil
}

func (s *Storage) GetSnapshot(pid, id string) ([]byte, error) {
	if !idRE.MatchString(id) {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(s.snapshotPath(pid, id))
}

// PutSnapshot stores a manifest after checking its id and that every blob
// and parent it references is already on the server.
func (s *Storage) PutSnapshot(pid, id string, data []byte) error {
	m, err := manifest.Parse(id, data)
	if err != nil {
		return err
	}
	for _, h := range m.Objects() {
		if !s.Objects.Has(h) {
			return fmt.Errorf("snapshot %s references missing object %s", id[:10], h[:10])
		}
	}
	for _, p := range m.Parents {
		if !s.HasSnapshot(pid, p) {
			return fmt.Errorf("snapshot %s references missing parent %s", id[:10], p[:min(10, len(p))])
		}
	}
	return os.WriteFile(s.snapshotPath(pid, id), data, 0o644)
}

// Branches are stored one file per branch (projects/<pid>/branches/<name>,
// body = version id), the same layout as the object-storage backend.
func (s *Storage) branchDir(pid string) string { return filepath.Join(s.projectDir(pid), "branches") }

// migrateBranches converts the older single branches.json file.
func (s *Storage) migrateBranches(pid string) error {
	old := filepath.Join(s.projectDir(pid), "branches.json")
	b := map[string]string{}
	if err := readJSON(old, &b); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(s.branchDir(pid), 0o755); err != nil {
		return err
	}
	for name, id := range b {
		if branchRE.MatchString(name) {
			if err := writeFileAtomic(filepath.Join(s.branchDir(pid), name), []byte(id+"\n")); err != nil {
				return err
			}
		}
	}
	return os.Rename(old, old+".migrated")
}

func (s *Storage) Branches(pid string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.migrateBranches(pid); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.branchDir(pid))
	b := map[string]string{}
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !branchRE.MatchString(e.Name()) || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		if id := s.branchHead(pid, e.Name()); id != "" {
			b[e.Name()] = id
		}
	}
	return b, nil
}

func (s *Storage) branchHead(pid, name string) string {
	data, err := os.ReadFile(filepath.Join(s.branchDir(pid), name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// UpdateBranch moves a branch from old to new atomically. old is "" for a
// new branch; new is "" to delete it.
func (s *Storage) UpdateBranch(pid, name, old, new string) error {
	if !branchRE.MatchString(name) || strings.HasSuffix(name, ".tmp") {
		return fmt.Errorf("invalid branch name %q", name)
	}
	if new != "" && !s.HasSnapshot(pid, new) {
		return fmt.Errorf("unknown snapshot %s", new)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.migrateBranches(pid); err != nil {
		return err
	}
	if current := s.branchHead(pid, name); current != old {
		return &ErrConflict{Current: current}
	}
	path := filepath.Join(s.branchDir(pid), name)
	if new == "" {
		return os.Remove(path)
	}
	if err := os.MkdirAll(s.branchDir(pid), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(new+"\n"))
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
