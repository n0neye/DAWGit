package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Workspaces are each member's unsaved work, reported by their agent: which
// tracks they are editing (for soft locks) and a backup of their sets. The
// server stores them as opaque JSON documents.

var wsRE = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (s *Storage) workspacePath(pid, wsid string) string {
	return filepath.Join(s.projectDir(pid), "workspaces", wsid+".json")
}

func (s *Storage) PutWorkspace(pid, wsid string, data []byte) error {
	if !wsRE.MatchString(wsid) {
		return fmt.Errorf("invalid workspace id")
	}
	var probe map[string]any
	if err := json.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	dir := filepath.Dir(s.workspacePath(pid, wsid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := s.workspacePath(pid, wsid) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.workspacePath(pid, wsid))
}

// Workspaces returns all workspace documents of a project.
func (s *Storage) Workspaces(pid string) ([]json.RawMessage, error) {
	dir := filepath.Join(s.projectDir(pid), "workspaces")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := []json.RawMessage{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && json.Valid(data) {
			out = append(out, data)
		}
	}
	return out, nil
}

func (h *handlers) putWorkspace(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.s.PutWorkspace(r.PathValue("pid"), r.PathValue("wsid"), data); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	ws, err := h.s.Workspaces(r.PathValue("pid"))
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONResponse(w, http.StatusOK, ws)
}
