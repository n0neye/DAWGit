package project

import (
	"fmt"
	"io"

	"dawgit/internal/profile"
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
