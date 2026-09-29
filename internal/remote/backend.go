package remote

import (
	"fmt"
	"io"
	"strings"
)

// Backend is where a team's projects are shared: the DAWGit server today,
// other storage later (see docs/design/storage-backends.md).
//
// File contents and version manifests are immutable and named by their
// SHA-256; writing one that exists is harmless. Branch heads are the only
// data several people change, so UpdateBranch is compare-and-swap. Workspace
// states each have a single writer.
//
// Clients must write file contents, then the manifest, then the branch, so a
// branch never points at a version whose data is missing.
type Backend interface {
	Projects() ([]Project, error)
	PutProject(p Project) error

	// Branches maps branch name to version id.
	Branches(pid string) (map[string]string, error)
	// UpdateBranch moves a branch from old to new: old == "" creates it,
	// new == "" deletes it. Returns *ErrConflict if the branch has moved.
	UpdateBranch(pid, name, old, new string) error

	MissingSnapshots(pid string, ids []string) ([]string, error)
	PutSnapshot(pid, id string, data []byte) error
	GetSnapshot(pid, id string) ([]byte, error)

	MissingObjects(hashes []string) ([]string, error)
	PutObject(hash string, r io.Reader) error
	// GetObject returns the contents; the caller closes it.
	GetObject(hash string) (io.ReadCloser, error)

	// PutWorkspace stores this workspace's state (JSON-encodable).
	PutWorkspace(pid, wsid string, state any) error
	// Workspaces decodes all workspace states into out (pointer to a slice).
	Workspaces(pid string, out any) error
}

var _ Backend = (*Client)(nil)

// Config selects and configures a backend (stored in .dawgit/config.json).
type Config struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

// Open returns the backend for a configuration.
func Open(cfg Config) (Backend, error) {
	u := strings.ToLower(cfg.URL)
	switch {
	case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"):
		return New(cfg.URL, cfg.Token), nil
	}
	return nil, fmt.Errorf("unsupported server address %q (expected http:// or https://)", cfg.URL)
}
