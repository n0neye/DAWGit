package remote

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"dawgit/internal/jsonx"
)

// Backend is where a team's projects are shared: a DAWGit server, or an
// S3-compatible bucket used directly (see docs/design/storage-backends.md).
//
// File contents and version manifests are immutable and named by their
// SHA-256; writing one that exists is harmless. Branch heads are the only
// data several people change, so UpdateBranch is compare-and-swap. Workspace
// states each have a single writer.
//
// Clients must write file contents, then the manifest, then the branch, so a
// branch never points at a version whose data is missing.
type Backend interface {
	// Info describes the team (its name); empty for backends that have none.
	Info() (TeamInfo, error)
	// SetInfo renames the team for everyone.
	SetInfo(info TeamInfo) error

	// Members lists the team's members (their ids and display names);
	// PutMember adds a member or renames one.
	Members() ([]Member, error)
	PutMember(m Member) error

	Projects() ([]Project, error)
	PutProject(p Project) error
	// DeleteProject removes a project from the team: its versions, branches
	// and workspaces. Stored files may stay until they are cleaned up.
	DeleteProject(pid string) error

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

// BodyStore is storage that keeps encoded blobs (package blob: compressed,
// with a header) under an object's hash; the upload is checked against the
// encoding's own SHA-256. A team server keeps contents as they are.
//
// A big file may be kept as pieces: its pieces are put as objects, then
// MarkChunked, then the chunk list (blob.ChunkList) under the file's hash
// (see docs/design/chunked-files.md).
type BodyStore interface {
	PutObjectBody(hash string, r io.Reader, size int64, bodySHA string) error
	// MarkChunked notes that the object hash is (about to be) a chunk list,
	// so storage cleanup keeps its pieces.
	MarkChunked(hash string) error
}

var _ BodyStore = (*S3Backend)(nil)

// Member is a person in the team. Versions record the member's id, so a new
// display name applies to everything they did.
type Member struct {
	ID   string `json:"id"` // 32 hex characters
	Name string `json:"name"`
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *Member) UnmarshalJSON(b []byte) error {
	type plain Member
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v Member) MarshalJSON() ([]byte, error) {
	type plain Member
	return jsonx.Encode(plain(v), v.Extra)
}

// ValidMemberID reports whether id looks like a member id.
func ValidMemberID(id string) bool { return validHex(id, 32) }

// TeamInfo describes a team as its server or storage names it.
type TeamInfo struct {
	Name string `json:"name"`
	// Features the team turned on (see CheckFeatures).
	Features []string `json:"features,omitempty"`
	// Extra: fields a newer DAWGit wrote, kept when this one rewrites the record.
	Extra jsonx.Extra `json:"-"`
}

// UnmarshalJSON and MarshalJSON keep fields this build doesn't know (see
// package jsonx).
func (v *TeamInfo) UnmarshalJSON(b []byte) error {
	type plain TeamInfo
	return jsonx.Decode(b, (*plain)(v), &v.Extra)
}

func (v TeamInfo) MarshalJSON() ([]byte, error) {
	type plain TeamInfo
	return jsonx.Encode(plain(v), v.Extra)
}

// Config selects and configures a backend (stored in .dawgit/config.json).
//
//	server:  URL "http(s)://host:port", Token
//	storage: URL "s3+https://endpoint-host/bucket/prefix" (s3+http for local
//	         testing), AccessKey, SecretKey, Region (default "auto")
type Config struct {
	URL       string `json:"url"`
	Token     string `json:"token,omitempty"`
	AccessKey string `json:"access_key,omitempty"`
	SecretKey string `json:"secret_key,omitempty"`
	Region    string `json:"region,omitempty"`
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

// IsStorage reports whether the config points at object storage (no server).
func (c Config) IsStorage() bool { return strings.HasPrefix(strings.ToLower(c.URL), "s3+") }

// PollInterval is how often an agent should check for changes: object
// storage has no push and is billed per request, so it is polled less often.
func (c Config) PollInterval() time.Duration {
	if c.IsStorage() {
		return time.Minute
	}
	return 5 * time.Second
}

// Display is the address without credentials, for showing to users.
func (c Config) Display() string { return c.URL }

// Opener opens a backend for a configuration (see Register).
type Opener func(cfg Config) (Backend, error)

var openers = map[string]Opener{}

// Register adds a kind of backend: addresses starting with prefix (e.g.
// "rtdb+https://") are opened by open. Registered prefixes are tried before
// the built-in server and storage backends.
func Register(prefix string, open Opener) { openers[strings.ToLower(prefix)] = open }

// Capabilities are features a backend offers beyond versions; the app shows
// what goes with them only when the team's backend has them.
type Capabilities struct {
	Locks    bool `json:"locks"`    // files can be locked for editing
	Presence bool `json:"presence"` // who is working on what, live
}

// Capable is implemented by backends with capabilities.
type Capable interface{ Capabilities() Capabilities }

// CapabilitiesOf a backend (none for the built-in ones).
func CapabilitiesOf(b Backend) Capabilities {
	if c, ok := b.(Capable); ok {
		return c.Capabilities()
	}
	return Capabilities{}
}

// Open returns the backend for a configuration.
func Open(cfg Config) (Backend, error) {
	u := strings.ToLower(cfg.URL)
	for prefix, open := range openers {
		if strings.HasPrefix(u, prefix) {
			return open(cfg)
		}
	}
	switch {
	case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"):
		return New(cfg.URL, cfg.Token), nil
	case strings.HasPrefix(u, "s3+http://"), strings.HasPrefix(u, "s3+https://"):
		p, err := url.Parse(cfg.URL[len("s3+"):])
		if err != nil {
			return nil, fmt.Errorf("invalid storage address: %w", err)
		}
		bucket, prefix, _ := strings.Cut(strings.Trim(p.Path, "/"), "/")
		return NewS3(p.Scheme+"://"+p.Host, bucket, prefix, cfg.Region, cfg.AccessKey, cfg.SecretKey)
	}
	return nil, fmt.Errorf("unsupported address %q (expected a server http(s):// address or a connection code)", cfg.URL)
}

// Connection codes bundle a storage config (including credentials) into one
// string that can be pasted into the app.
const codePrefix = "dawgit-s3:"

// EncodeConnectionCode packs cfg into a connection code.
func EncodeConnectionCode(cfg Config) string {
	data, _ := json.Marshal(cfg)
	return codePrefix + base64.RawURLEncoding.EncodeToString(data)
}

// IsConnectionCode reports whether s looks like a connection code.
func IsConnectionCode(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), codePrefix) }

// ParseAddress turns what a user typed into a Config: a connection code, or
// a server address plus token.
func ParseAddress(addr, token string) (Config, error) {
	addr = strings.TrimSpace(addr)
	if !IsConnectionCode(addr) {
		return Config{URL: strings.TrimRight(addr, "/"), Token: strings.TrimSpace(token)}, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(addr, codePrefix))
	var cfg Config
	if err == nil {
		err = json.Unmarshal(data, &cfg)
	}
	if err != nil || !cfg.IsStorage() {
		return Config{}, errors.New("invalid connection code")
	}
	return cfg, nil
}
