package project

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/profile"
	"dawgit/internal/store"
	"dawgit/internal/version"
)

// Profile is the project's rules (.dawgit.yaml, or detected from the folder).
// With a broken .dawgit.yaml it returns the detected rules and the error:
// the project can be looked at, but not committed (see checkProfile).
func (r *Repo) Profile() (*profile.Profile, error) {
	if r.prof == nil {
		r.prof, r.profErr = profile.Load(r.Root)
	}
	return r.prof, r.profErr
}

// rules is the profile for deciding what is tracked (errors aside).
func (r *Repo) rules() *profile.Profile {
	p, _ := r.Profile()
	return p
}

// CheckRules reports why the project's rules can't be followed (nil when
// they can): a broken .dawgit.yaml, or one for a newer DAWGit.
func (r *Repo) CheckRules() error { return r.checkProfile() }

// checkProfile refuses to commit or take in versions with a .dawgit.yaml
// this DAWGit can't follow: everyone must track the same files.
func (r *Repo) checkProfile() error {
	p, err := r.Profile()
	if err != nil {
		return fmt.Errorf("fix the project's rules first: %w", err)
	}
	return needsNewer(p)
}

func needsNewer(p *profile.Profile) error {
	if v := p.NeedsNewer(version.Version); v != "" {
		return fmt.Errorf("this project's %s needs DAWGit %s or later (this is %s): update DAWGit first",
			profile.FileName, v, version.Version)
	}
	return nil
}

// profileOf is the rules a version was committed with. A broken file gives
// the detected rules (one bad commit must not block the team); a file
// needing a newer DAWGit is an error.
func (r *Repo) profileOf(m *Manifest) (*profile.Profile, error) {
	f, ok := m.FileMap()[profile.FileName]
	if !ok {
		return profile.Detect(r.Root), nil
	}
	if err := r.ensureHashes([]string{f.Hash}); err != nil {
		return nil, err
	}
	rc, err := r.openObject(f.Hash)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	p, err := profile.Parse(data, r.Root)
	if err != nil {
		return p, nil
	}
	return p, needsNewer(p)
}

// forgetProfile makes the next Profile read .dawgit.yaml again (e.g. after a
// checkout replaced it).
func (r *Repo) forgetProfile() { r.prof, r.profErr = nil, nil }

// EnsureRules makes the project's .dawgit.yaml the one place its rules are
// in: written with what DAWGit finds when there is none, and given the
// presets DAWGit finds when it doesn't say which apply (an older file). It
// returns what it did ("" nothing, "created", "presets added"). A broken
// file is left alone (fixing it is the person's).
func (r *Repo) EnsureRules() (string, error) {
	p := filepath.Join(r.Root, profile.FileName)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		if err := store.WriteAtomic(p, strings.NewReader(profile.Generate(r.Root))); err != nil {
			return "", err
		}
		r.forgetProfile()
		return "created", nil
	}
	if err != nil {
		return "", err
	}
	text, err := profile.WithFoundPresets(string(data), r.Root)
	if err != nil || text == string(data) {
		return "", nil
	}
	if err := store.WriteAtomic(p, strings.NewReader(text)); err != nil {
		return "", err
	}
	r.forgetProfile()
	return "presets added", nil
}

// SetPreset sets which preset applies to folder ("" or "." the project's;
// "none" for no preset) in the project's .dawgit.yaml, keeping the rest of
// the file as it is.
func (r *Repo) SetPreset(folder, preset string) error {
	if _, ok := profile.Builtin(preset); !ok && preset != "none" {
		return fmt.Errorf("%q is not a preset DAWGit knows", preset)
	}
	if folder == "." {
		folder = ""
	}
	p := filepath.Join(r.Root, profile.FileName)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		data, err = []byte(profile.Generate(r.Root)), nil
	}
	if err != nil {
		return err
	}
	text, err := profile.SetPreset(string(data), folder, preset, false)
	if err != nil {
		return err
	}
	if _, err := profile.Parse([]byte(text), r.Root); err != nil {
		return fmt.Errorf("the change would break %s: %w", profile.FileName, err)
	}
	if err := store.WriteAtomic(p, strings.NewReader(text)); err != nil {
		return err
	}
	r.forgetProfile()
	return nil
}
