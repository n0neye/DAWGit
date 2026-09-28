// Package project manages a DAWGit repository inside an Ableton project
// folder: working-file scanning, snapshots, status, log and checkout.
package project

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"dawgit/internal/store"
)

const metaDir = ".dawgit"

type Config struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name,omitempty"`
	Author    string `json:"author"`
	// Branch this workspace follows; empty means "main".
	Branch string        `json:"branch,omitempty"`
	Remote *RemoteConfig `json:"remote,omitempty"`
	// WorkspaceID identifies this copy of the project to the server.
	WorkspaceID string `json:"workspace_id,omitempty"`
}

type RemoteConfig struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

type Repo struct {
	Root   string // Ableton project folder
	Dir    string // Root/.dawgit
	Store  *store.Store
	Config Config
}

// ErrNotRepo is returned when no .dawgit directory is found.
var ErrNotRepo = errors.New("not a dawgit project (run `dawgit init` in an Ableton project folder)")

// Init creates a repository in root, which must look like an Ableton project.
func Init(root, author string) (*Repo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(root, metaDir)); err == nil {
		return nil, fmt.Errorf("%s is already a dawgit project", root)
	}
	if !looksLikeProject(root) {
		return nil, fmt.Errorf("%s does not look like an Ableton project (no .als file or \"Ableton Project Info\")", root)
	}
	id := make([]byte, 16)
	rand.Read(id)
	if author == "" {
		author = defaultAuthor()
	}
	return create(root, Config{ProjectID: hex.EncodeToString(id), Name: projectName(root), Author: author})
}

// create lays out .dawgit in root with the given config.
func create(root string, cfg Config) (*Repo, error) {
	r := &Repo{Root: root, Dir: filepath.Join(root, metaDir), Config: cfg}
	for _, d := range []string{"objects", "snapshots"} {
		if err := os.MkdirAll(filepath.Join(r.Dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	if err := r.SaveConfig(); err != nil {
		return nil, err
	}
	var err error
	r.Store, err = store.Open(filepath.Join(r.Dir, "objects"))
	return r, err
}

func (r *Repo) SaveConfig() error {
	return writeJSON(filepath.Join(r.Dir, "config.json"), r.Config)
}

// projectName derives a display name from an Ableton project folder
// ("My Song Project" -> "My Song").
func projectName(root string) string {
	return strings.TrimSuffix(filepath.Base(root), " Project")
}

// BranchName is the branch this workspace follows.
func (r *Repo) BranchName() string {
	if r.Config.Branch == "" {
		return "main"
	}
	return r.Config.Branch
}

// Open finds the repository containing dir (searching upward).
func Open(dir string) (*Repo, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, metaDir)); err == nil && fi.IsDir() {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNotRepo
		}
		dir = parent
	}
	r := &Repo{Root: dir, Dir: filepath.Join(dir, metaDir)}
	if err := readJSON(filepath.Join(r.Dir, "config.json"), &r.Config); err != nil {
		return nil, err
	}
	r.Store, err = store.Open(filepath.Join(r.Dir, "objects"))
	return r, err
}

func looksLikeProject(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "Ableton Project Info")); err == nil {
		return true
	}
	sets, _ := filepath.Glob(filepath.Join(root, "*.als"))
	return len(sets) > 0
}

func defaultAuthor() string {
	if a := os.Getenv("DAWGIT_AUTHOR"); a != "" {
		return a
	}
	if u, err := user.Current(); err == nil {
		name := u.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
		return name
	}
	return "unknown"
}

// Abs converts a slash-separated project-relative path to an OS path.
func (r *Repo) Abs(rel string) string { return filepath.Join(r.Root, filepath.FromSlash(rel)) }

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return store.WriteAtomic(path, strings.NewReader(string(data)+"\n"))
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
