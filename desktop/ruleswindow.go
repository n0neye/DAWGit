package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dawgit/internal/profile"
	"dawgit/internal/project"
)

// The Rules window: a project's .dawgit.yaml shown and changed with folders,
// switches and menus.

// PresetOption is a preset the window offers.
type PresetOption struct {
	Name    string   `json:"name"`
	LeftOut []string `json:"leftOut"` // its ignore patterns
}

// RuleItem is a rule of .dawgit.yaml.
type RuleItem struct {
	Kind    string `json:"kind"` // ignore | track
	Pattern string `json:"pattern"`
}

// RulesDetail is what the window shows of a project's rules.
type RulesDetail struct {
	Presets     []profile.PresetEntry `json:"presets"`
	Rules       []RuleItem            `json:"rules"`
	Suggestions []RuleSuggestion      `json:"suggestions"`
	Options     []PresetOption        `json:"options"`
	Error       string                `json:"error"` // the file can't be read: fix it as text
}

// ProjectRules reads a project's rules for the Rules window (writing the
// file first, when missing).
func (a *App) ProjectRules(root string) (*RulesDetail, error) {
	if !knownProject(root) {
		return nil, errors.New("unknown project")
	}
	unlock := a.lock(root)
	defer unlock()
	r, err := project.Open(root)
	if err != nil {
		return nil, err
	}
	if _, err := r.EnsureRules(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, profile.FileName))
	if err != nil {
		return nil, err
	}
	out := &RulesDetail{Presets: profile.PresetEntries(string(data)), Rules: []RuleItem{},
		Suggestions: suggestions(r), Options: []PresetOption{}}
	for _, name := range profile.Names() {
		p, _ := profile.Builtin(name)
		out.Options = append(out.Options, PresetOption{Name: name, LeftOut: append([]string{}, p.Ignore...)})
	}
	rules, err := r.Profile()
	if err != nil {
		out.Error = err.Error()
		return out, nil
	}
	for _, ru := range rules.Rules {
		if ru.Ignore != "" {
			out.Rules = append(out.Rules, RuleItem{Kind: "ignore", Pattern: ru.Ignore})
		} else {
			out.Rules = append(out.Rules, RuleItem{Kind: "track", Pattern: ru.Track})
		}
	}
	return out, nil
}

// RuleNode is a file or folder in the Rules window's tree.
type RuleNode struct {
	Name    string `json:"name"`
	Path    string `json:"path"` // relative to the project folder
	Dir     bool   `json:"dir"`
	Ignored bool   `json:"ignored"`
	By      string `json:"by"`     // the rule or preset that decides
	Preset  string `json:"preset"` // a folder presets: names: its preset
	Size    int64  `json:"size"`   // files only
}

// RulesFolder lists a folder of the project (rel; "" the project folder)
// with whether each file and folder is tracked, and why.
func (a *App) RulesFolder(root, rel string) ([]RuleNode, error) {
	if !knownProject(root) {
		return nil, errors.New("unknown project")
	}
	rules, _ := profile.Load(root)
	presets := map[string]string{}
	if data, err := os.ReadFile(filepath.Join(root, profile.FileName)); err == nil {
		for _, e := range profile.PresetEntries(string(data)) {
			presets[strings.ToLower(e.Folder)] = e.Preset
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	out := []RuleNode{}
	for _, e := range entries {
		p := e.Name()
		if rel != "" {
			p = rel + "/" + e.Name()
		}
		if p == ".dawgit" {
			continue
		}
		d := rules.Explain(p, e.IsDir())
		n := RuleNode{Name: e.Name(), Path: p, Dir: e.IsDir(), Ignored: d.Ignored, By: d.By}
		if e.IsDir() {
			n.Preset = presets[strings.ToLower(p)]
		} else if fi, err := e.Info(); err == nil {
			n.Size = fi.Size()
		}
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// SetTracked tracks a file or folder of the project, or leaves it out, with
// the fewest rules: a rule of the window's that said the opposite goes; a
// rule is added only when still needed.
func (a *App) SetTracked(root, rel string, dir, tracked bool) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == profile.FileName {
		return errors.New("that one can't be left out")
	}
	unlock := a.lock(root)
	defer unlock()
	path := filepath.Join(root, profile.FileName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data, err = []byte(profile.Generate(root)), nil
	}
	if err != nil {
		return err
	}
	text := string(data)
	pattern := "/" + globQuote(rel)
	if dir {
		pattern += "/"
	}
	// The rule for exactly this path that says the opposite: removed.
	opposite := "track"
	if tracked {
		opposite = "ignore"
	}
	rules, err := profile.Parse([]byte(text), root)
	if err != nil {
		return fmt.Errorf("fix %s first: %w", profile.FileName, err)
	}
	for i := len(rules.Rules) - 1; i >= 0; i-- {
		ru := rules.Rules[i]
		if (opposite == "ignore" && ru.Ignore == pattern) || (opposite == "track" && ru.Track == pattern) {
			if text, err = profile.RemoveRule(text, i); err != nil {
				return err
			}
		}
	}
	if after, err := profile.Parse([]byte(text), root); err == nil && after.Ignored(rel, dir) == tracked {
		kind := "ignore"
		if tracked {
			kind = "track"
		}
		if text, err = profile.AddRule(text, kind, pattern); err != nil {
			return err
		}
	}
	if _, err := profile.Parse([]byte(text), root); err != nil {
		return fmt.Errorf("the change would break %s: %w", profile.FileName, err)
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

// RemoveRule removes the index-th rule of the project's .dawgit.yaml.
func (a *App) RemoveRule(root string, index int) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	unlock := a.lock(root)
	defer unlock()
	path := filepath.Join(root, profile.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text, err := profile.RemoveRule(string(data), index)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
