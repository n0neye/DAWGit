package desktop

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"dawgit/internal/profile"
	"dawgit/internal/project"
)

// OpenRules opens the project's .dawgit.yaml in a text editor (written with
// what DAWGit finds first, when missing).
func (a *App) OpenRules(root string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	unlock := a.lock(root)
	r, err := project.Open(root)
	if err == nil {
		_, err = r.EnsureRules()
	}
	unlock()
	if err != nil {
		return err
	}
	return shellEdit(filepath.Join(root, profile.FileName))
}

// RuleSuggestion is a project of a tool found in a folder the rules don't
// name yet, with how much its preset would leave out there.
type RuleSuggestion struct {
	profile.Suggestion
	LeftOutBytes int64 `json:"leftOutBytes"` // -1: not counted (too many files)
}

// leftOutCache: bytes a suggestion would leave out, by root|folder|preset
// (counting walks the folder once).
var leftOutCache sync.Map

// suggestions are the rules' suggestions for a project, with sizes.
func suggestions(r *project.Repo) []RuleSuggestion {
	out := []RuleSuggestion{}
	rules, err := r.Profile()
	if err != nil {
		return out
	}
	for _, s := range rules.Suggestions() {
		key := r.Root + "|" + s.Folder + "|" + s.Preset
		n, ok := leftOutCache.Load(key)
		if !ok {
			n = leftOutBytes(r.Root, rules, s)
			leftOutCache.Store(key, n)
		}
		out = append(out, RuleSuggestion{Suggestion: s, LeftOutBytes: n.(int64)})
	}
	return out
}

// leftOutBytes counts what taking the suggestion would leave out that the
// rules track now (at most 200,000 files looked at).
func leftOutBytes(root string, rules *profile.Profile, s profile.Suggestion) int64 {
	text, err := os.ReadFile(filepath.Join(root, profile.FileName))
	if err != nil {
		return -1
	}
	changed, err := profile.SetPreset(string(text), s.Folder, s.Preset, false)
	if err != nil {
		return -1
	}
	after, err := profile.Parse([]byte(changed), root)
	if err != nil {
		return -1
	}
	var total int64
	seen := 0
	base := filepath.Join(root, filepath.FromSlash(s.Folder))
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > 200000 {
			return errors.New("too many files")
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == ".dawgit" {
			if rel == ".dawgit" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if rules.Ignored(rel, true) && after.Ignored(rel, true) {
				return filepath.SkipDir // left out either way
			}
			return nil
		}
		if after.Ignored(rel, false) && !rules.Ignored(rel, false) {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	if err != nil {
		return -1
	}
	return total
}

// SetPreset says which preset applies to a folder of a project ("none": it
// isn't a project of that tool), in its .dawgit.yaml.
func (a *App) SetPreset(root, folder, preset string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	unlock := a.lock(root)
	defer unlock()
	r, err := project.Open(root)
	if err != nil {
		return err
	}
	return r.SetPreset(folder, preset)
}

// AddIgnoreRule adds `- ignore: pattern` at the end of the rules in the
// project's .dawgit.yaml (creating the file): the last rule wins. The file
// is committed with the project, so the rule is the team's once shared.
func (a *App) AddIgnoreRule(root, pattern string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	unlock := a.lock(root)
	defer unlock()
	p := filepath.Join(root, profile.FileName)
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		data, err = []byte(profile.Generate(root)), nil
	}
	if err != nil {
		return err
	}
	text, err := withIgnoreRule(string(data), pattern)
	if err != nil {
		return err
	}
	if _, err := profile.Parse([]byte(text), root); err != nil {
		return fmt.Errorf("the rule would break %s: %w", profile.FileName, err)
	}
	return os.WriteFile(p, []byte(text), 0o644)
}

// withIgnoreRule adds `- ignore: pattern` at the end of the rules.
func withIgnoreRule(text, pattern string) (string, error) {
	return profile.AddRule(text, "ignore", pattern)
}

// globQuote makes a name match itself in a rule: *, ?, [ and \ are taken
// literally.
func globQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`*?[\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// IgnoreOption is a rule offered for leaving a file or folder out.
type IgnoreOption struct {
	Label   string `json:"label"`
	Pattern string `json:"pattern"`
}

// IgnoreOptions are the rules offered for leaving a file or folder out:
// just it, or all like it (same extension, or folders of that name).
func (a *App) IgnoreOptions(rel string, isDir bool) []IgnoreOption {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == profile.FileName {
		return []IgnoreOption{}
	}
	name := rel[strings.LastIndex(rel, "/")+1:]
	if isDir {
		return []IgnoreOption{
			{Label: "This folder", Pattern: "/" + globQuote(rel) + "/"},
			{Label: fmt.Sprintf("All folders named “%s”", name), Pattern: globQuote(name) + "/"},
		}
	}
	out := []IgnoreOption{{Label: "This file", Pattern: "/" + globQuote(rel)}}
	if ext := filepath.Ext(name); ext != "" && ext != name {
		out = append(out, IgnoreOption{Label: fmt.Sprintf("All %s files", ext), Pattern: "*" + globQuote(ext)})
	}
	return out
}
