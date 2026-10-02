package profile

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PresetsVersion is the oldest DAWGit that reads presets: (generated files
// require it). A constant, not this DAWGit's version: everyone's DAWGit
// writes the same file for the same folder, so two people adding it at once
// don't conflict.
const PresetsVersion = "0.9.4"

// FoundMark ends the lines DAWGit wrote from what it found in the folder.
const FoundMark = "# found by DAWGit"

// Generate is the .dawgit.yaml DAWGit writes for root: the presets it finds
// (marked as found), no rules yet, and the rules people ask for most,
// commented out. The same folder always gives the same text.
func Generate(root string) string {
	var b strings.Builder
	b.WriteString(`# DAWGit's rules for this project: which files are left out of versions.
# Committed with the project, so the whole team uses the same rules.
# Guide: https://github.com/n0neye/DAWGit/blob/main/docs/profiles.md
requires: "` + PresetsVersion + `"
# Which preset applies to which folder: Live's, Unity's... own files left out.
# "none": no preset there.
presets:
`)
	found := Detect(root).Applied()
	if len(found) == 0 {
		b.WriteString("  ./: none\n")
	}
	for _, a := range found {
		b.WriteString(presetLine(a.Folder, a.Preset, true) + "\n")
	}
	b.WriteString(`rules:
  # Later rules win. Ignored files stay on everyone's disk.
  # - ignore: "Exports/"    # leave a folder out of versions
  # - track: "*.wav"        # keep files a preset leaves out after all
`)
	return b.String()
}

// folderKey is how presets: names a folder ("./" for the project's).
func folderKey(folder string) string {
	if folder == "" {
		return "./"
	}
	return strconv.Quote(folder + "/")
}

func presetLine(folder, preset string, found bool) string {
	line := "  " + folderKey(folder) + ": " + preset
	if found {
		line += "  " + FoundMark
	}
	return line
}

// SetPreset sets the preset of folder ("" the project's; "none" for no
// preset) in the text of a .dawgit.yaml, keeping everything else (comments
// included). An older use: block is renamed presets:. found marks the line
// as DAWGit's finding.
func SetPreset(text, folder, preset string, found bool) (string, error) {
	folder = cleanFolder(folder)
	if strings.TrimSpace(preset) == "" || strings.ContainsAny(preset+folder, "\n\r\"") {
		return "", errors.New("bad preset or folder")
	}
	crlf := strings.Contains(text, "\r\n")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	join := func(ls []string) string {
		out := strings.Join(ls, "\n")
		if crlf {
			out = strings.ReplaceAll(out, "\n", "\r\n")
		}
		return out
	}
	line := presetLine(folder, preset, found)
	at := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "presets:") || strings.HasPrefix(l, "use:") {
			at = i
			break
		}
	}
	if at < 0 { // no presets yet: before the rules, or at the end
		ins := len(lines)
		for ins > 0 && strings.TrimSpace(lines[ins-1]) == "" {
			ins--
		}
		for i, l := range lines {
			if strings.HasPrefix(l, "rules:") {
				ins = i
				break
			}
		}
		block := []string{"presets:", line}
		return join(append(lines[:ins], append(block, lines[ins:]...)...)), nil
	}
	head := strings.TrimSpace(strings.SplitN(lines[at], ":", 2)[1])
	if head != "" && !strings.HasPrefix(head, "#") {
		return "", errors.New("write the presets one per line to change one here")
	}
	lines[at] = "presets:" + strings.TrimPrefix(strings.TrimPrefix(lines[at], "presets:"), "use:")
	end := at + 1
	for i := at + 1; i < len(lines); i++ {
		l := lines[i]
		if l != "" && l[0] != ' ' && l[0] != '\t' && l[0] != '#' {
			break // the next key
		}
		if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, "#") {
			if k, ok := entryFolder(t); ok && strings.EqualFold(k, folder) {
				lines[i] = line
				return join(lines), nil
			}
			end = i + 1
		}
	}
	return join(append(lines[:end], append([]string{line}, lines[end:]...)...)), nil
}

// entryFolder is the folder a presets: line names.
func entryFolder(line string) (string, bool) {
	key := line
	if strings.HasPrefix(line, `"`) {
		end := strings.Index(line[1:], `"`)
		if end < 0 {
			return "", false
		}
		unq, err := strconv.Unquote(line[:end+2])
		if err != nil {
			return "", false
		}
		key = unq
	} else if i := strings.Index(line, ":"); i >= 0 {
		key = line[:i]
	} else {
		return "", false
	}
	return cleanFolder(key), true
}

// Suggestion is a project of a tool found in a folder the file's presets:
// don't name: its preset is suggested (or "none", to say it isn't one).
type Suggestion struct {
	Folder string `json:"folder"` // "" the project folder
	Preset string `json:"preset"`
	// LeftOut: what the preset leaves out there (its ignore patterns).
	LeftOut []string `json:"leftOut"`
}

// Suggestions lists the presets detection finds in folders the file's
// presets: don't name (none without presets:, where detection applies).
func (p *Profile) Suggestions() []Suggestion {
	if p.Named == nil {
		return nil
	}
	found := &Profile{root: p.root, Rules: p.Rules, Gitignore: p.Gitignore, gi: p.gi,
		applied: append([]applied(nil), p.applied...)}
	names := Names()
	sort.SliceStable(names, func(i, j int) bool { return builtin[names[i]].Priority > builtin[names[j]].Priority })
	if !p.Named[""] {
		for _, name := range names {
			if detects(builtin[name], p.root) {
				found.applied = append(found.applied, applied{Applied: Applied{Folder: "", Preset: name}, preset: builtin[name]})
				break
			}
		}
	}
	found.detectInside(names)
	var out []Suggestion
	for _, a := range found.applied {
		if p.Named[strings.ToLower(a.Folder)] || a.preset == nil {
			continue
		}
		out = append(out, Suggestion{Folder: a.Folder, Preset: a.Preset, LeftOut: append([]string{}, a.preset.Ignore...)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Folder < out[j].Folder })
	return out
}

// String describes a suggestion for people.
func (s Suggestion) String() string {
	where := "the project folder"
	if s.Folder != "" {
		where = s.Folder + "/"
	}
	return fmt.Sprintf("%s looks like a %s project", where, s.Preset)
}

// WithFoundPresets adds what detection finds in root to a .dawgit.yaml
// without presets: (an older file), marked as found, and raises its
// requires: to PresetsVersion. A file with presets: is returned as it is.
func WithFoundPresets(text, root string) (string, error) {
	p, err := Parse([]byte(text), root)
	if err != nil {
		return "", err
	}
	if p.Named != nil {
		return text, nil
	}
	found := Detect(root).Applied()
	if len(found) == 0 {
		found = []Applied{{Folder: "", Preset: "none"}}
	}
	for _, a := range found {
		if text, err = SetPreset(text, a.Folder, a.Preset, a.Preset != "none"); err != nil {
			return "", err
		}
	}
	if p.NeedsNewer(PresetsVersion) == "" && p.Requires != PresetsVersion {
		text = setRequires(text, PresetsVersion)
	}
	return text, nil
}

// setRequires sets requires: (adding it first when missing).
func setRequires(text, v string) string {
	crlf := strings.Contains(text, "\r\n")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	line := `requires: "` + v + `"`
	done := false
	for i, l := range lines {
		if strings.HasPrefix(l, "requires:") {
			lines[i], done = line, true
			break
		}
	}
	if !done {
		at := 0
		for at < len(lines) && strings.HasPrefix(lines[at], "#") {
			at++
		}
		lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
	}
	out := strings.Join(lines, "\n")
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out
}
