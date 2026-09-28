// Package manifest defines the snapshot manifest shared by client and server.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type FileEntry struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// Manifest describes one snapshot. Its id is the SHA-256 of its encoding.
type Manifest struct {
	Version int      `json:"version"`
	Parents []string `json:"parents"`
	Author  string   `json:"author"`
	Time    string   `json:"time"`
	Message string   `json:"message"`
	// Files inside the project folder.
	Files []FileEntry `json:"files"`
	// External are samples referenced by a set from outside the project,
	// keyed by their original absolute path (slash separated).
	External []FileEntry `json:"external,omitempty"`
	// Packs are Live packs whose samples are referenced (not stored).
	Packs []string `json:"packs,omitempty"`
	// Missing are referenced samples that were not found when snapshotting.
	Missing []string `json:"missing,omitempty"`

	ID string `json:"-"`
}

// Encode returns the canonical bytes whose hash is the snapshot id.
func (m *Manifest) Encode() []byte {
	data, _ := json.MarshalIndent(m, "", "  ")
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

// Parse decodes a manifest and checks it against its id.
func Parse(id string, data []byte) (*Manifest, error) {
	if got := ID(data); got != id {
		return nil, fmt.Errorf("manifest %s: content hash is %s", short(id), short(got))
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
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

// Objects lists every blob hash the snapshot needs.
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

func short(id string) string { return id[:min(10, len(id))] }
