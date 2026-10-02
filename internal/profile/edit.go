package profile

import (
	"errors"
	"strconv"
	"strings"
)

// Editing a .dawgit.yaml as text, line by line, so everything else in it
// (comments, order, blank lines) stays as people wrote it.

type textLines struct {
	lines []string
	crlf  bool
}

func splitText(text string) *textLines {
	return &textLines{lines: strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n"),
		crlf: strings.Contains(text, "\r\n")}
}

func (t *textLines) String() string {
	out := strings.Join(t.lines, "\n")
	if t.crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out
}

// block finds a top-level key ("rules:"): its line, and the lines of its
// items (from, to). at < 0 when it isn't there.
func (t *textLines) block(key string) (at, from, to int) {
	at = -1
	for i, l := range t.lines {
		if strings.HasPrefix(l, key) {
			at = i
			break
		}
	}
	if at < 0 {
		return -1, 0, 0
	}
	from, to = at+1, at+1
	for i := at + 1; i < len(t.lines); i++ {
		l := t.lines[i]
		if l != "" && l[0] != ' ' && l[0] != '\t' && l[0] != '-' && l[0] != '#' {
			break // the next key
		}
		if strings.TrimSpace(l) != "" { // items and their comments
			to = i + 1
		}
	}
	return at, from, to
}

// ruleLine reads a rules: item ("- ignore: "x"").
func ruleLine(l string) (kind, pattern string, ok bool) {
	s := strings.TrimSpace(l)
	if !strings.HasPrefix(s, "- ") {
		return "", "", false
	}
	s = strings.TrimSpace(s[2:])
	for _, k := range []string{"ignore", "track"} {
		if strings.HasPrefix(s, k+":") {
			v := strings.TrimSpace(s[len(k)+1:])
			if i := strings.Index(v, " #"); i >= 0 && !strings.HasPrefix(v, `"`) {
				v = strings.TrimSpace(v[:i])
			}
			if strings.HasPrefix(v, `"`) {
				if end := strings.LastIndex(v, `"`); end > 0 {
					if u, err := strconv.Unquote(v[:end+1]); err == nil {
						v = u
					}
				}
			}
			return k, v, true
		}
	}
	return "", "", false
}

// AddRule adds `- kind: pattern` (ignore or track) at the end of the rules
// (the last rule wins); the same rule there already is moved to the end.
func AddRule(text, kind, pattern string) (string, error) {
	if kind != "ignore" && kind != "track" {
		return "", errors.New("a rule is ignore or track")
	}
	if strings.TrimSpace(pattern) == "" || strings.ContainsAny(pattern, "\n\r") {
		return "", errors.New("empty rule")
	}
	item := "- " + kind + ": " + strconv.Quote(pattern)
	t := splitText(text)
	at, from, to := t.block("rules:")
	if at < 0 { // no rules yet
		for len(t.lines) > 0 && strings.TrimSpace(t.lines[len(t.lines)-1]) == "" {
			t.lines = t.lines[:len(t.lines)-1]
		}
		t.lines = append(t.lines, "rules:", "  "+item, "")
		return t.String(), nil
	}
	rest := strings.TrimSpace(strings.TrimPrefix(t.lines[at], "rules:"))
	if strings.HasPrefix(rest, "[") { // rules: [] (an empty list written inline)
		if strings.TrimSpace(strings.SplitN(rest, "#", 2)[0]) != "[]" {
			return "", errors.New("write the rules one per line to add one here")
		}
		t.lines[at] = "rules:"
	}
	indent := "  "
	for i := from; i < to; i++ {
		if k, p, ok := ruleLine(t.lines[i]); ok {
			l := t.lines[i]
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
			if k == kind && p == pattern && i == to-1 {
				return t.String(), nil // the last rule already
			}
		}
	}
	t.lines = append(append(append([]string{}, t.lines[:to]...), indent+item), t.lines[to:]...)
	// The same rule earlier: it no longer decides anything, drop it.
	for i := from; i < to; i++ {
		if k, p, ok := ruleLine(t.lines[i]); ok && k == kind && p == pattern {
			t.lines = append(t.lines[:i], t.lines[i+1:]...)
			break
		}
	}
	return t.String(), nil
}

// RemoveRule removes the index-th rule (0 first, as in Profile.Rules).
func RemoveRule(text string, index int) (string, error) {
	t := splitText(text)
	at, from, to := t.block("rules:")
	if at < 0 {
		return "", errors.New("no rules")
	}
	n := 0
	for i := from; i < to; i++ {
		if _, _, ok := ruleLine(t.lines[i]); ok {
			if n == index {
				t.lines = append(t.lines[:i], t.lines[i+1:]...)
				return t.String(), nil
			}
			n++
		}
	}
	return "", errors.New("no such rule")
}

// PresetEntry is a line of presets:.
type PresetEntry struct {
	Folder string `json:"folder"` // "" the project folder
	Preset string `json:"preset"`
	Found  bool   `json:"found"` // DAWGit wrote it from what it found
}

// PresetEntries reads the presets: (or use:) lines of a .dawgit.yaml.
func PresetEntries(text string) []PresetEntry {
	t := splitText(text)
	at, from, to := t.block("presets:")
	if at < 0 {
		at, from, to = t.block("use:")
	}
	out := []PresetEntry{}
	if at < 0 {
		return out
	}
	for i := from; i < to; i++ {
		s := strings.TrimSpace(t.lines[i])
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		folder, ok := entryFolder(s)
		if !ok {
			continue
		}
		// what follows the key: "preset  # comment"
		v := s
		if strings.HasPrefix(s, `"`) {
			v = s[strings.Index(s[1:], `"`)+2:]
		}
		v = strings.TrimSpace(v[strings.Index(v, ":")+1:])
		if j := strings.Index(v, "#"); j >= 0 {
			v = strings.TrimSpace(v[:j])
		}
		out = append(out, PresetEntry{Folder: folder, Preset: strings.Trim(v, `"'`), Found: strings.HasSuffix(s, FoundMark)})
	}
	return out
}
