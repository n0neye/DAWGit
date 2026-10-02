package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dawgit/internal/profile"
	"dawgit/internal/version"
)

// rulesFile starts a project's .dawgit.yaml: what it is, the preset DAWGit
// found for the folder (none: what it finds then), and the two rules people
// ask for most, commented out.
func rulesFile(root string) string {
	// This DAWGit's version (major.minor) is the oldest that follows it.
	v := version.Version
	if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
		v = parts[0] + "." + parts[1]
	}
	use := ""
	if a := profile.Detect(root).Applied(); len(a) > 0 {
		use = "use:\n"
		for _, x := range a {
			folder := "./"
			if x.Folder != "" {
				folder = strconv.Quote(x.Folder + "/") // a project found inside
			}
			use += "  " + folder + ": " + x.Preset + "\n"
		}
	}
	return `# DAWGit's rules for this project: which files are left out of versions.
# This file is committed with the project, so the whole team uses the same rules.
# Guide: https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md
requires: "` + v + `"
` + use + `rules:
  # Later rules win. Ignored files stay on everyone's disk.
  # - ignore: "Exports/"    # leave a folder out of versions
  # - track: "*.wav"        # keep files a preset leaves out after all
`
}

// OpenRules opens the project's .dawgit.yaml in a text editor, creating it
// from a commented template first.
func (a *App) OpenRules(root string) error {
	if !knownProject(root) {
		return errors.New("unknown project")
	}
	p := filepath.Join(root, profile.FileName)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(p, []byte(rulesFile(root)), 0o644); err != nil {
			return err
		}
	}
	return shellEdit(p)
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
		data, err = []byte(rulesFile(root)), nil
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

// withIgnoreRule inserts the rule after the last line of the rules block,
// keeping everything else (comments included) as it is.
func withIgnoreRule(text, pattern string) (string, error) {
	if strings.TrimSpace(pattern) == "" || strings.ContainsAny(pattern, "\n\r") {
		return "", errors.New("empty rule")
	}
	item := "- ignore: " + strconv.Quote(pattern)
	crlf := strings.Contains(text, "\r\n")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	join := func(ls []string) string {
		out := strings.Join(ls, "\n")
		if crlf {
			out = strings.ReplaceAll(out, "\n", "\r\n")
		}
		return out
	}
	at := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "rules:") {
			at = i
			break
		}
	}
	if at < 0 { // no rules yet
		for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines = lines[:len(lines)-1]
		}
		return join(append(lines, "rules:", "  "+item, "")), nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(lines[at], "rules:"))
	if strings.HasPrefix(rest, "[") { // rules: [] (an empty list written inline)
		if strings.TrimSpace(strings.SplitN(rest, "#", 2)[0]) != "[]" {
			return "", errors.New("write the rules one per line to add one here")
		}
		lines[at] = "rules:"
	}
	indent, end := "  ", at+1
	for i := at + 1; i < len(lines); i++ {
		l := lines[i]
		if l != "" && l[0] != ' ' && l[0] != '\t' && l[0] != '-' {
			break // the next key
		}
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		end = i + 1
		if strings.HasPrefix(t, "- ") && !strings.HasPrefix(t, "#") {
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			if strings.Contains(t, strconv.Quote(pattern)) && strings.HasPrefix(t, "- ignore:") {
				return join(lines), nil // there already
			}
		}
	}
	out := append(append(append([]string{}, lines[:end]...), indent+item), lines[end:]...)
	return join(out), nil
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
