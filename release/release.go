// Package release signs DAWGit installers and checks the signatures: an
// update is installed only when its installer is the one the publisher
// signed. Public (not internal) so builds with extensions sign theirs too.
//
// A release's manifest names its version, the installer's SHA-256, the
// oldest version that may keep working with the team (older ones must
// update), and the publisher's ed25519 signature over all three.
package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Manifest is what a release says about its installer.
type Manifest struct {
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`               // the installer's, hex
	MinVersion string `json:"minVersion,omitempty"` // older versions must update
	Signature  string `json:"signature"`            // base64
}

func (m Manifest) message() []byte {
	return []byte("dawgit-release/1\n" + m.Version + "\n" + strings.ToLower(m.SHA256) + "\n" + m.MinVersion + "\n")
}

// Sign signs m with the publisher's key.
func Sign(key ed25519.PrivateKey, m *Manifest) {
	m.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, m.message()))
}

// Verify checks m's signature with the publisher's public key (base64).
func Verify(publicKey string, m Manifest) error {
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("no release key in this build")
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(pub, m.message(), sig) {
		return errors.New("the update's signature doesn't match")
	}
	return nil
}

// FileSHA256 hashes a file (hex).
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// KeyPath is where the publisher's signing key lives: DAWGIT_SIGNING_KEY, or
// a file in the user's config folder (never in a repository).
func KeyPath() string {
	if p := os.Getenv("DAWGIT_SIGNING_KEY"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "DAWGit-release", "signing.key")
}

// LoadKey reads the signing key (its 32-byte seed, base64).
func LoadKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("signing key: %w (make one with: dawgit-release keygen)", err)
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("signing key: not a key")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// NewKey makes a signing key at path (refusing to replace one) and returns
// its public half (base64), to build into the app.
func NewKey(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s exists already", path)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(pub), nil
}

// PublicKey is the public half of a signing key (base64).
func PublicKey(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}
