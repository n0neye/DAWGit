// Package teams keeps the teams this computer is connected to, with their
// credentials, in one per-user file (so tokens and storage keys never live
// inside project folders, which people zip and share). It also remembers the
// user's name and where each team project was downloaded.
//
// File: %APPDATA%\DAWGit\teams.json (or $DAWGIT_CONFIG_DIR/teams.json).
package teams

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/remote"
)

type Team struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Remote remote.Config `json:"remote"` // includes credentials
	// CustomName is set when the user renamed the team on this computer;
	// otherwise Name follows the name the team's server or storage gives.
	CustomName bool `json:"customName,omitempty"`
	// MemberID and MemberName: who this computer is in the team (versions
	// record the id; the name is kept in the team's member list).
	MemberID   string `json:"memberId,omitempty"`
	MemberName string `json:"memberName,omitempty"`
}

type Store struct {
	// Author is the name shown on versions this user saves.
	Author  string `json:"author,omitempty"`
	Current string `json:"current,omitempty"` // selected team id
	Teams   []Team `json:"teams"`
	// Projects maps "<team id>/<project id>" to the local project folder.
	Projects map[string]string `json:"projects"`
	// Local lists project folders kept on this computer only (no team).
	Local []string `json:"local,omitempty"`

	path string
}

// Dir is where DAWGit keeps per-user settings.
func Dir() string {
	if d := os.Getenv("DAWGIT_CONFIG_DIR"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "DAWGit")
}

// Load reads the store; a missing file is an empty store.
func Load() (*Store, error) {
	s := &Store{path: filepath.Join(Dir(), "teams.json"), Projects: map[string]string{}}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Projects == nil {
		s.Projects = map[string]string{}
	}
	for i, t := range s.Teams {
		if old := oldDefaultName(t.Remote); old != "" && t.Name == old {
			s.Teams[i].Name = DefaultName(t.Remote)
		}
	}
	return s, nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := s.path + ".tmp"
	// 0600: the file holds tokens and storage keys.
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// NormalizeURL makes team addresses comparable.
func NormalizeURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

func (s *Store) Find(id string) *Team {
	for i := range s.Teams {
		if s.Teams[i].ID == id {
			return &s.Teams[i]
		}
	}
	return nil
}

// FindByURL returns the team with this address, or nil.
func (s *Store) FindByURL(raw string) *Team {
	n := NormalizeURL(raw)
	for i := range s.Teams {
		if NormalizeURL(s.Teams[i].Remote.URL) == n {
			return &s.Teams[i]
		}
	}
	return nil
}

// Upsert adds a team or updates the credentials of the one with the same
// address. name is used when the team is new (or unnamed).
func (s *Store) Upsert(cfg remote.Config, name string) *Team {
	cfg.URL = NormalizeURL(cfg.URL)
	if t := s.FindByURL(cfg.URL); t != nil {
		t.Remote = cfg
		if t.Name == "" {
			t.Name = DefaultName(cfg)
		}
		s.SyncName(t.ID, name)
		return t
	}
	b := make([]byte, 8)
	rand.Read(b)
	if name == "" {
		name = DefaultName(cfg)
	}
	s.Teams = append(s.Teams, Team{ID: hex.EncodeToString(b), Name: name, Remote: cfg})
	if s.Current == "" {
		s.Current = s.Teams[len(s.Teams)-1].ID
	}
	return &s.Teams[len(s.Teams)-1]
}

// SyncName takes the name the team's server or storage gives (empty: none),
// unless the user renamed the team here. It reports whether it changed.
func (s *Store) SyncName(id, teamName string) bool {
	t := s.Find(id)
	if t == nil || t.CustomName || teamName == "" || t.Name == teamName {
		return false
	}
	t.Name = teamName
	return true
}

// Rename names a team on this computer only. An empty name goes back to the
// team's own name (the default until the next sync).
func (s *Store) Rename(id, name string) error {
	t := s.Find(id)
	if t == nil {
		return errors.New("unknown team")
	}
	if name = strings.TrimSpace(name); name == "" {
		t.CustomName = false
		t.Name = DefaultName(t.Remote)
		return nil
	}
	t.Name, t.CustomName = name, true
	return nil
}

// DefaultName is shown when neither the user nor the server named the team.
func DefaultName(cfg remote.Config) string {
	u, err := url.Parse(strings.TrimPrefix(cfg.URL, "s3+"))
	if err != nil || u.Host == "" {
		return cfg.URL
	}
	if cfg.IsStorage() {
		bucket, _, _ := strings.Cut(strings.Trim(u.Path, "/"), "/")
		return bucket
	}
	return u.Host
}

// oldDefaultName is what DefaultName returned before 0.2 for storage.
func oldDefaultName(cfg remote.Config) string {
	u, err := url.Parse(strings.TrimPrefix(cfg.URL, "s3+"))
	if err != nil || !cfg.IsStorage() {
		return ""
	}
	return "Storage " + strings.Trim(u.Path, "/")
}

// NewID returns n random bytes as hex (team and member ids).
func NewID(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// LocalID as Current selects the projects kept on this computer only.
const LocalID = "local"

// Remove forgets a team and its project locations (folders stay on disk).
func (s *Store) Remove(id string) {
	out := s.Teams[:0]
	for _, t := range s.Teams {
		if t.ID != id {
			out = append(out, t)
		}
	}
	s.Teams = out
	for k := range s.Projects {
		if strings.HasPrefix(k, id+"/") {
			delete(s.Projects, k)
		}
	}
	if s.Current == id {
		s.Current = ""
		if len(s.Teams) > 0 {
			s.Current = s.Teams[0].ID
		}
	}
}

func (s *Store) ProjectRoot(teamID, projectID string) string {
	return s.Projects[teamID+"/"+projectID]
}

func (s *Store) SetProjectRoot(teamID, projectID, root string) {
	s.Projects[teamID+"/"+projectID] = root
	s.RemoveLocal(root)
}

func (s *Store) ForgetProject(teamID, projectID string) {
	delete(s.Projects, teamID+"/"+projectID)
}

// Roots lists every known project folder (all teams and local-only).
func (s *Store) Roots() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range s.Projects {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range s.Local {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) AddLocal(root string) {
	for _, r := range s.Local {
		if r == root {
			return
		}
	}
	s.Local = append(s.Local, root)
	sort.Strings(s.Local)
}

func (s *Store) RemoveLocal(root string) {
	out := s.Local[:0]
	for _, r := range s.Local {
		if r != root {
			out = append(out, r)
		}
	}
	s.Local = out
}
