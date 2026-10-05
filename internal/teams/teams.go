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
	"time"

	"dawgit/internal/flock"
	"dawgit/internal/jsonx"
	"dawgit/internal/remote"
	"dawgit/internal/version"
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
	// ShareSetup: this computer shares its setup (Live versions, plugins,
	// packs: names only) with the team, for project checks.
	ShareSetup bool `json:"shareSetup,omitempty"`
	// SetupAsked: the user chose whether to share it (on or off); until
	// then the app asks once.
	SetupAsked bool `json:"setupAsked,omitempty"`
	// Backup: this computer backs up the team's storage (nil: it doesn't).
	Backup *Backup `json:"backup,omitempty"`
	// BackupHushed: when the user last put off the reminder to set one up.
	BackupHushed time.Time `json:"backupHushed,omitempty"`
	// KeysUnreadable: the team's keys were sealed by another Windows user
	// or on another computer (teams.json copied): connect again with the
	// team's connection code.
	KeysUnreadable bool `json:"-"`
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *Team) UnmarshalJSON(b []byte) error {
	type plain Team
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v Team) MarshalJSON() ([]byte, error) {
	type plain Team
	return jsonx.Encode(plain(v), v.Extra)
}

// Backup is where and how this computer backs up a team (internal/backup).
type Backup struct {
	// Where: a folder, or S3-compatible storage (its keys sealed like the
	// team's).
	Folder      string         `json:"folder,omitempty"`
	Storage     *remote.Config `json:"storage,omitempty"`
	Paused      bool           `json:"paused,omitempty"`
	LastSuccess time.Time      `json:"lastSuccess,omitempty"`
	LastAttempt time.Time      `json:"lastAttempt,omitempty"`
	LastError   string         `json:"lastError,omitempty"`
	Size        int64          `json:"size,omitempty"` // bytes, at the last success
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *Backup) UnmarshalJSON(b []byte) error {
	type plain Backup
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v Backup) MarshalJSON() ([]byte, error) {
	type plain Backup
	return jsonx.Encode(plain(v), v.Extra)
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
	// ManualUpdates: DAWGit doesn't install updates on its own (it still
	// downloads them and offers them).
	ManualUpdates bool `json:"manualUpdates,omitempty"`
	// Channel: the release line updates come from, "stable" or "nightly";
	// "" for the one this build is from. Stable and Nightly share this file:
	// what either writes, the other reads.
	Channel string `json:"channel,omitempty"`

	path   string
	locked bool // inside Update: the lock is held
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *Store) UnmarshalJSON(b []byte) error {
	type plain Store
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v Store) MarshalJSON() ([]byte, error) {
	type plain Store
	return jsonx.Encode(plain(v), v.Extra)
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
	// A build with extensions keeps its own teams and settings
	// (%APPDATA%\DAWGit Pro), so it can run next to the public app.
	return filepath.Join(d, version.Name())
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
		rc := &s.Teams[i].Remote
		for _, secret := range []*string{&rc.Token, &rc.AccessKey, &rc.SecretKey} {
			plain, err := unsealSecret(*secret)
			if err != nil {
				plain = ""
				s.Teams[i].KeysUnreadable = true
			}
			*secret = plain
		}
		if b := s.Teams[i].Backup; b != nil && b.Storage != nil {
			for _, secret := range []*string{&b.Storage.AccessKey, &b.Storage.SecretKey} {
				plain, err := unsealSecret(*secret)
				if err != nil {
					plain = "" // the backup fails, saying the keys were refused
				}
				*secret = plain
			}
		}
	}
	return s, nil
}

// Update changes the store without losing anyone's change: it takes the
// settings lock, reads the store fresh, lets fn change it and saves it (an
// error from fn saves nothing). The app, the command line tool and
// background work (backups, setups) all change teams.json; a plain Load and
// Save could write back an old copy over another's change.
func Update(fn func(*Store) error) (*Store, error) {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return nil, err
	}
	unlock, err := flock.Lock(filepath.Join(Dir(), "teams.lock"), 15*time.Second)
	if err != nil {
		return nil, err
	}
	defer unlock()
	s, err := Load()
	if err != nil {
		return nil, err
	}
	s.locked = true
	defer func() { s.locked = false }()
	if err := fn(s); err != nil {
		return nil, err
	}
	return s, s.Save()
}

// Save writes the store as it is. To change it, use Update: Save alone
// writes back whatever this copy holds, even over a newer change.
func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if !s.locked {
		unlock, err := flock.Lock(filepath.Join(filepath.Dir(s.path), "teams.lock"), 15*time.Second)
		if err != nil {
			return err
		}
		defer unlock()
	}
	// Secrets sealed in the file, plain in memory.
	sealed := *s
	sealed.Teams = make([]Team, len(s.Teams))
	for i, t := range s.Teams {
		t.Remote.Token = sealSecret(t.Remote.Token)
		t.Remote.AccessKey = sealSecret(t.Remote.AccessKey)
		t.Remote.SecretKey = sealSecret(t.Remote.SecretKey)
		if t.Backup != nil && t.Backup.Storage != nil {
			b, st := *t.Backup, *t.Backup.Storage
			st.AccessKey, st.SecretKey = sealSecret(st.AccessKey), sealSecret(st.SecretKey)
			b.Storage = &st
			t.Backup = &b
		}
		sealed.Teams[i] = t
	}
	data, _ := json.MarshalIndent(&sealed, "", "  ")
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

// SyncTeamName records the name a team gives itself now (see SyncName),
// saving only when it changed; it says whether it did.
func SyncTeamName(id, teamName string) bool {
	if s, err := Load(); err != nil || !s.SyncName(id, teamName) {
		return false
	}
	changed := false
	Update(func(s *Store) error {
		changed = s.SyncName(id, teamName)
		return nil
	})
	return changed
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

// ImportFromPro gives a first start without settings DAWGit Pro's (its
// users move to DAWGit's Nightly channel): teams, keys, where projects are,
// names. Nothing happens once this computer has DAWGit settings, or with
// DAWGIT_CONFIG_DIR set.
func ImportFromPro() (bool, error) {
	if os.Getenv("DAWGIT_CONFIG_DIR") != "" || version.Edition != "" {
		return false, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return false, nil
	}
	return importFrom(filepath.Join(base, "DAWGit Pro", "teams.json"), filepath.Join(Dir(), "teams.json"))
}

func importFrom(from, to string) (bool, error) {
	if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	data, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return false, err
	}
	// Keys stay sealed as they are: same Windows user, same computer.
	if err := os.WriteFile(to, data, 0o600); err != nil {
		return false, err
	}
	_, err = Update(func(s *Store) error {
		s.Channel = "nightly"
		return nil
	})
	return true, err
}

// Open connects to the team: its backend, checked for the team's features
// (a team that turned on one this build lacks fails with
// *remote.ErrTeamFeatures). Everything that works with a connected team
// opens it here.
func (t Team) Open() (remote.Backend, error) {
	b, err := remote.Open(t.Remote)
	if err != nil {
		return nil, err
	}
	if err := remote.CheckFeatures(b, t.Remote.URL); err != nil {
		return nil, err
	}
	return b, nil
}
