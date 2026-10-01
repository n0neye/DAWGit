// Package manifest defines the snapshot manifest shared by client and server.
//
// Format 1 lists every file in the version record. Format 2 (current) stores
// the project folder as trees, one per folder (see tree.go and
// docs/design/tree-manifests.md); the record names the top tree. In memory a
// manifest always has the flat list of files.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type FileEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Format is the version record format this code writes; it reads every
// format up to it.
const Format = 2

// ErrNewerFormat: the version was made by a newer DAWGit.
var ErrNewerFormat = errors.New("this version was saved by a newer DAWGit: update DAWGit to open it")

// Manifest describes one snapshot. Its id is the SHA-256 of its encoding.
type Manifest struct {
	Version int      `json:"version"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"` // the name at the time (older versions: the only record)
	// AuthorID is the team member's id; their current name comes from the
	// team's member list. Empty for versions from before member ids.
	AuthorID string `json:"author_id,omitempty"`
	Time     string `json:"time"`
	Message  string `json:"message"`
	// Files inside the project folder. Format 2 records hold Tree instead:
	// Files is then filled from the trees (nil in a header).
	Files []FileEntry `json:"files"`
	// Tree is the project folder's tree (format 2).
	Tree string `json:"-"`
	// FileCount and TotalSize: how many files, how many bytes (format 2,
	// known without reading the trees).
	FileCount int   `json:"-"`
	TotalSize int64 `json:"-"`
	// External are samples referenced by a set from outside the project,
	// keyed by their original absolute path (slash separated).
	External []FileEntry `json:"external,omitempty"`
	// Packs are Live packs whose samples are referenced (not stored).
	Packs []string `json:"packs,omitempty"`
	// Missing are referenced samples that were not found when snapshotting.
	Missing []string `json:"missing,omitempty"`

	ID string `json:"-"`
}

// record2 is a format-2 record as stored. "files" is the number of files:
// a DAWGit that only knows format 1 (a list there) fails to read it rather
// than taking the version for an empty project.
type record2 struct {
	Version  int         `json:"version"`
	Parents  []string    `json:"parents"`
	Author   string      `json:"author"`
	AuthorID string      `json:"author_id,omitempty"`
	Time     string      `json:"time"`
	Message  string      `json:"message"`
	Files    int         `json:"files"`
	Size     int64       `json:"size"`
	Tree     string      `json:"tree"`
	External []FileEntry `json:"external,omitempty"`
	Packs    []string    `json:"packs,omitempty"`
	Missing  []string    `json:"missing,omitempty"`
}

// Encode returns the canonical bytes whose hash is the snapshot id: format
// 2 when Tree is set, format 1 otherwise.
func (m *Manifest) Encode() []byte {
	var data []byte
	if m.Version >= 2 {
		data, _ = json.MarshalIndent(record2{Version: m.Version, Parents: m.Parents, Author: m.Author,
			AuthorID: m.AuthorID, Time: m.Time, Message: m.Message, Files: m.FileCount, Size: m.TotalSize,
			Tree: m.Tree, External: m.External, Packs: m.Packs, Missing: m.Missing}, "", "  ")
	} else {
		data, _ = json.MarshalIndent(m, "", "  ")
	}
	return append(data, '\n')
}

// Seal computes and sets the id; call after the manifest is complete.
func (m *Manifest) Seal() []byte {
	data := m.Encode()
	m.ID = ID(data)
	return data
}

func ID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Parse decodes a manifest and checks it against its id. A format-2
// manifest comes without Files (read its trees for them).
func Parse(id string, data []byte) (*Manifest, error) {
	if got := ID(data); got != id {
		return nil, fmt.Errorf("manifest %s: content hash is %s", short(id), short(got))
	}
	var head struct {
		Version int             `json:"version"`
		Files   json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", short(id), err)
	}
	if head.Version > Format {
		return nil, fmt.Errorf("version %s: %w", short(id), ErrNewerFormat)
	}
	m := &Manifest{}
	if head.Version >= 2 {
		var r record2
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("manifest %s: %w", short(id), err)
		}
		if !validHash(r.Tree) {
			return nil, fmt.Errorf("manifest %s: bad tree %q", short(id), r.Tree)
		}
		*m = Manifest{Version: r.Version, Parents: r.Parents, Author: r.Author, AuthorID: r.AuthorID,
			Time: r.Time, Message: r.Message, Tree: r.Tree, FileCount: r.Files, TotalSize: r.Size,
			External: r.External, Packs: r.Packs, Missing: r.Missing}
	} else if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", short(id), err)
	}
	m.ID = id
	return m, nil
}

func (m *Manifest) FileMap() map[string]FileEntry {
	out := make(map[string]FileEntry, len(m.Files))
	for _, f := range m.Files {
		out[f.Path] = f
	}
	return out
}

// Objects lists every blob hash the snapshot needs (not its trees).
func (m *Manifest) Objects() []string {
	var out []string
	for _, f := range m.Files {
		out = append(out, f.Hash)
	}
	for _, f := range m.External {
		out = append(out, f.Hash)
	}
	return out
}

func validHash(h string) bool {
	if len(h) != 64 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

func short(id string) string { return id[:min(10, len(id))] }
