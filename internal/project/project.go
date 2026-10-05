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
	"sync"

	"dawgit/internal/jsonx"
	"dawgit/internal/profile"
	"dawgit/internal/remote"
	"dawgit/internal/store"
	"dawgit/internal/teams"
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
	// Tip is the latest version of the branch while the project is on an
	// older one (see GoTo); empty otherwise.
	Tip string `json:"tip,omitempty"`
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *Config) UnmarshalJSON(b []byte) error {
	type plain Config
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v Config) MarshalJSON() ([]byte, error) {
	type plain Config
	return jsonx.Encode(plain(v), v.Extra)
}

// RemoteConfig selects the team's backend (server address and token).
type RemoteConfig = remote.Config

type Repo struct {
	Root   string // Ableton project folder
	Dir    string // Root/.dawgit
	Store  *store.Store
	Config Config

	// OnProgress, when set, hears about long steps (see Progress).
	OnProgress func(Progress)

	// Only, when not nil, limits the next commits to the changes of these
	// paths (from Status); other changes stay uncommitted.
	Only []string

	sizes   map[string]int64  // object sizes known from manifests (knowSizes)
	remote  map[string]bool   // objects only in the team's storage (remoteOnly)
	sources map[string]string // contents found outside the store (sourcesByHash)
	stamps  map[string]stamp  // the project files among them: as they were hashed
	srcMu   sync.Mutex        // localCopy runs in parallel transfers
	pinned  map[string]string // contents a step keeps outside the store (see pin)
	prof    *profile.Profile  // the project's rules (Profile)
	profErr error
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
		if kind := profile.NightlyKind(root); kind != "" {
			return nil, &profile.NeedsNightly{Kind: kind}
		}
		return nil, fmt.Errorf("%s does not look like a project DAWGit knows (for Ableton Live: a .als file or "+
			"\"Ableton Project Info\"; or add a %s)", root, profile.FileName)
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

// Rename gives the project another name, for its team too (the folder keeps
// its own).
func (r *Repo) Rename(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return errors.New("a name needs 1 to 100 characters")
	}
	if r.Config.Remote != nil && r.Config.Remote.URL != "" {
		c, err := r.Client()
		if err != nil {
			return err
		}
		if err := remote.RenameProject(c, r.Config.ProjectID, name); err != nil {
			return err
		}
	}
	r.Config.Name = name
	return r.SaveConfig()
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

// looksLikeProject: a preset recognizes the folder, or it has its own rules.
func looksLikeProject(root string) bool {
	if _, err := os.Stat(filepath.Join(root, profile.FileName)); err == nil {
		return true
	}
	return len(profile.Detect(root).Applied()) > 0
}

// Identity is who commits here: in a team, the member chosen on this computer
// (id and name); otherwise the name set on this computer, or the project's.
func (r *Repo) Identity() (id, name string) {
	store, _ := teams.Load()
	if store != nil && r.Config.Remote != nil {
		if t := store.FindByURL(r.Config.Remote.URL); t != nil && t.MemberID != "" {
			return t.MemberID, t.MemberName
		}
	}
	if a := os.Getenv("DAWGIT_AUTHOR"); a != "" {
		return "", a
	}
	if store != nil && store.Author != "" {
		return "", store.Author
	}
	return "", r.Config.Author
}

// MemberNames maps the team's member ids to their current names (empty for
// projects without a team, or when the team cannot be reached).
func (r *Repo) MemberNames() map[string]string {
	names := map[string]string{}
	c, err := r.Client()
	if err != nil {
		return names
	}
	ms, err := c.Members()
	if err != nil {
		return names
	}
	for _, m := range ms {
		names[m.ID] = m.Name
	}
	return names
}

// AuthorName is the name to show for a version: the member's current name
// when known, else the name recorded with it.
func AuthorName(m *Manifest, names map[string]string) string {
	if n := names[m.AuthorID]; n != "" {
		return n
	}
	return m.Author
}

func defaultAuthor() string {
	if a := os.Getenv("DAWGIT_AUTHOR"); a != "" {
		return a
	}
	if s, err := teams.Load(); err == nil && s.Author != "" {
		return s.Author
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
