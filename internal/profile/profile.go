// Package profile decides how DAWGit treats each file of a project: which
// files are left out of versions, which tool's handlers apply (e.g. Live
// Sets merged track by track), how files are grouped in the app.
//
// A project may have a .dawgit.yaml in its folder; it is versioned with the
// project, so the whole team uses the same rules:
//
//	requires: "0.7"           # oldest DAWGit that understands this file
//	use:
//	  ./: ableton             # which preset applies to which folder
//	rules:                    # yours, above every preset; later ones win
//	  - ignore: "**/Exports/"
//	  - track: "**/*.asd"     # undo an ignore from a preset
//
// Without the file the preset is detected from the folder (an Ableton
// project uses the built-in "ableton" preset).
package profile

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the profile's file in the project folder.
const FileName = ".dawgit.yaml"

//go:embed presets/*.yaml
var presetFiles embed.FS

// Preset is what DAWGit knows about one creative tool.
type Preset struct {
	Name string `yaml:"name"`
	// Tool is the program's name in the app ("Ableton Live", "Unity").
	Tool string `yaml:"tool"`
	// Open: what "Open in <tool>" offers, and with which opener.
	Open     Open                `yaml:"open"`
	Checks   []string            `yaml:"checks"` // run before a commit (see handlers)
	Detect   []string            `yaml:"detect"`
	Ignore   []string            `yaml:"ignore"`
	Handlers []Handler           `yaml:"handlers"`
	Running  string              `yaml:"running"`
	Kinds    map[string][]string `yaml:"kinds"`
	// Gitignore: follow the project's .gitignore files too.
	Gitignore bool `yaml:"gitignore"`
	// Fallback: detected only when no other preset is (e.g. a .gitignore
	// says "code", unless the folder is also a Unity project).
	Fallback bool `yaml:"fallback"`
}

// Open is what the app offers to open in the tool: files matching Files in
// the preset's folder ("." for the folder itself), opened by the opener With
// (a handler; "" for the file's own program).
type Open struct {
	Files []string `yaml:"files"`
	With  string   `yaml:"with"`
}

// Handler names built-in code that handles some files (a preset can only
// pick handlers; new ones need a new DAWGit).
type Handler struct {
	Files   []string `yaml:"files"`
	Merge   string   `yaml:"merge"`
	Samples string   `yaml:"samples"`
}

// Rule is one line of rules: exactly one of Ignore and Track.
type Rule struct {
	Ignore string `yaml:"ignore"`
	Track  string `yaml:"track"`
}

func (r Rule) pattern() string {
	if r.Ignore != "" {
		return r.Ignore
	}
	return r.Track
}

// file is the shape of .dawgit.yaml.
type file struct {
	Requires  string            `yaml:"requires"`
	Use       map[string]string `yaml:"use"`
	Rules     []Rule            `yaml:"rules"`
	Gitignore bool              `yaml:"gitignore"`
}

// Applied is a preset in use for a folder ("" is the project folder).
type Applied struct {
	Folder   string `json:"folder"`
	Preset   string `json:"preset"`
	Detected bool   `json:"detected"` // no .dawgit.yaml: found from the folder
}

// Profile is the resolved rules of a project.
type Profile struct {
	Requires string
	Rules    []Rule
	applied  []applied // longest folder first
	FromFile bool      // read from .dawgit.yaml (not detected)
	// Gitignore: the project's .gitignore files apply too (from
	// .dawgit.yaml, or a preset in use).
	Gitignore bool
	root      string
	gi        gitignores
}

type applied struct {
	Applied
	preset *Preset
}

var builtin = map[string]*Preset{}

func init() {
	entries, _ := presetFiles.ReadDir("presets")
	for _, e := range entries {
		data, _ := presetFiles.ReadFile("presets/" + e.Name())
		if err := RegisterPreset(data); err != nil {
			panic(fmt.Sprintf("built-in preset %s: %v", e.Name(), err))
		}
	}
}

// RegisterPreset adds a preset (YAML, as in presets/) to the ones DAWGit
// knows; a preset with the same name replaces it. Call it before projects
// are opened (e.g. from an extension's init).
func RegisterPreset(data []byte) error {
	var p Preset
	if err := strictUnmarshal(data, &p); err != nil {
		return err
	}
	if p.Name == "" || p.Name == "none" {
		return fmt.Errorf("a preset needs a name (not %q)", p.Name)
	}
	builtin[p.Name] = &p
	return nil
}

// Builtin returns a built-in preset by name.
func Builtin(name string) (*Preset, bool) {
	p, ok := builtin[name]
	return p, ok
}

func strictUnmarshal(data []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)                                            // a misspelt field is an error, not silently ignored
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) { // an empty file is fine
		return err
	}
	return nil
}

// Load reads root's .dawgit.yaml, or detects the profile when there is none.
// With a broken file it returns the detected profile and the error, so the
// project still shows what changed while committing is refused.
func Load(root string) (*Profile, error) {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return Detect(root), nil
	}
	if err != nil {
		return Detect(root), err
	}
	return Parse(data, root)
}

// Parse reads the contents of a .dawgit.yaml (root is used for detecting
// presets of folders the file doesn't name).
func Parse(data []byte, root string) (*Profile, error) {
	var f file
	if err := strictUnmarshal(data, &f); err != nil {
		return Detect(root), fmt.Errorf("%s: %s", FileName, plainYAMLError(err))
	}
	p := &Profile{Requires: strings.TrimSpace(f.Requires), Rules: f.Rules, FromFile: true, root: root,
		Gitignore: f.Gitignore}
	if p.Requires != "" {
		if _, err := parseVersion(p.Requires); err != nil {
			return Detect(root), fmt.Errorf("%s: requires: %q is not a version like \"0.7\"", FileName, p.Requires)
		}
	}
	for i, r := range f.Rules {
		if (r.Ignore == "") == (r.Track == "") {
			return Detect(root), fmt.Errorf("%s: rule %d: write either ignore: or track: (one per rule)", FileName, i+1)
		}
		if err := checkPattern(r.pattern()); err != nil {
			return Detect(root), fmt.Errorf("%s: rule %d: %w", FileName, i+1, err)
		}
	}
	if len(f.Use) == 0 {
		p.applied = Detect(root).applied
	}
	for folder, name := range f.Use {
		folder = cleanFolder(folder)
		if name == "none" {
			p.applied = append(p.applied, applied{Applied: Applied{Folder: folder, Preset: "none"}})
			continue
		}
		preset, ok := builtin[name]
		if !ok {
			return Detect(root), fmt.Errorf("%s: use: %q is not a preset DAWGit knows (known: %s)", FileName, name, strings.Join(Names(), ", "))
		}
		p.applied = append(p.applied, applied{Applied: Applied{Folder: folder, Preset: name}, preset: preset})
	}
	p.sort()
	p.followGitignore()
	return p, nil
}

// followGitignore: a preset in use may ask for the .gitignore files.
func (p *Profile) followGitignore() {
	for _, a := range p.applied {
		if a.preset != nil && a.preset.Gitignore {
			p.Gitignore = true
		}
	}
}

var (
	unknownField = regexp.MustCompile(`line (\d+): field (\S+) not found in type \S+`)
	yamlPrefix   = regexp.MustCompile(`^yaml: (unmarshal errors:\s*)?`)
)

// plainYAMLError rewords the YAML library's errors for people: "line 5:
// unknown field "ignor"" instead of naming Go types.
func plainYAMLError(err error) string {
	msg := yamlPrefix.ReplaceAllString(err.Error(), "")
	msg = unknownField.ReplaceAllString(msg, `line $1: unknown field "$2"`)
	return strings.Join(strings.Fields(msg), " ")
}

// Detect finds the preset of a project without .dawgit.yaml.
func Detect(root string) *Profile {
	p := &Profile{root: root}
	// Specific presets first, then fallbacks (see Preset.Fallback).
	for _, fallback := range []bool{false, true} {
		for _, name := range Names() {
			if builtin[name].Fallback == fallback && detects(builtin[name], root) {
				p.applied = append(p.applied, applied{Applied: Applied{Folder: "", Preset: name, Detected: true}, preset: builtin[name]})
				p.followGitignore()
				return p
			}
		}
	}
	return p
}

func detects(pr *Preset, root string) bool {
	for _, d := range pr.Detect {
		if strings.HasSuffix(d, "/") {
			if fi, err := os.Stat(filepath.Join(root, strings.TrimSuffix(d, "/"))); err == nil && fi.IsDir() {
				return true
			}
			continue
		}
		if m, _ := filepath.Glob(filepath.Join(root, d)); len(m) > 0 {
			return true
		}
	}
	return false
}

// Names lists the built-in presets.
func Names() []string {
	var out []string
	for n := range builtin {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func cleanFolder(f string) string {
	f = strings.Trim(filepath.ToSlash(strings.TrimSpace(f)), "/")
	f = strings.TrimPrefix(f, "./")
	if f == "." {
		f = ""
	}
	return f
}

func (p *Profile) sort() {
	sort.SliceStable(p.applied, func(i, j int) bool { return len(p.applied[i].Folder) > len(p.applied[j].Folder) })
}

// Applied lists the presets in use, by folder.
func (p *Profile) Applied() []Applied {
	out := []Applied{}
	for _, a := range p.applied {
		out = append(out, a.Applied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Folder < out[j].Folder })
	return out
}

// presetFor is the preset of the nearest folder containing rel, and rel
// relative to that folder.
func (p *Profile) presetFor(rel string) (*applied, string) {
	for i := range p.applied {
		a := &p.applied[i]
		if a.Folder == "" {
			return a, rel
		}
		if strings.EqualFold(rel, a.Folder) {
			return a, ""
		}
		if len(rel) > len(a.Folder) && strings.EqualFold(rel[:len(a.Folder)+1], a.Folder+"/") {
			return a, rel[len(a.Folder)+1:]
		}
	}
	return nil, rel
}

// Decision says whether a path is tracked and which rule decided it.
type Decision struct {
	Ignored bool   `json:"ignored"`
	By      string `json:"by"` // e.g. `rule 2: ignore "**/Exports/"`
}

// Explain decides whether rel (slash separated, relative to the project
// folder) is left out of versions, and why.
func (p *Profile) Explain(rel string, isDir bool) Decision {
	if why := core(rel, isDir); why != "" {
		return Decision{Ignored: true, By: why}
	}
	if rel == FileName {
		return Decision{By: FileName + " is always tracked"}
	}
	for i := len(p.Rules) - 1; i >= 0; i-- {
		r := p.Rules[i]
		if matchPath(r.pattern(), rel, isDir) {
			if r.Ignore != "" {
				return Decision{Ignored: true, By: fmt.Sprintf("rule %d: ignore %q", i+1, r.Ignore)}
			}
			return Decision{By: fmt.Sprintf("rule %d: track %q", i+1, r.Track)}
		}
	}
	if p.Gitignore {
		if d, ok := p.gitignored(rel, isDir); ok {
			return d
		}
	}
	if a, sub := p.presetFor(rel); a != nil && a.preset != nil && sub != "" {
		for _, pat := range a.preset.Ignore {
			if matchPath(pat, sub, isDir) {
				return Decision{Ignored: true, By: fmt.Sprintf("preset %s: ignore %q", a.Preset, pat)}
			}
		}
	}
	return Decision{By: "no rule leaves it out"}
}

// Ignored: rel is left out of versions.
func (p *Profile) Ignored(rel string, isDir bool) bool { return p.Explain(rel, isDir).Ignored }

// SkipDir: nothing inside the folder rel can be tracked, so scanning it can
// stop there. (With track rules a file inside an ignored folder may still be
// tracked, so only DAWGit's own folders are skipped then.)
func (p *Profile) SkipDir(rel string) bool {
	if core(rel, true) != "" {
		return true
	}
	for _, r := range p.Rules {
		if r.Track != "" {
			return false
		}
	}
	return p.Ignored(rel, true)
}

// core is what is always left out, whatever the rules say: DAWGit's own
// folder and temporary files, other version control, OS litter.
func core(rel string, isDir bool) string {
	segs := strings.Split(rel, "/")
	if segs[0] == ".dawgit" {
		return "DAWGit's own folder"
	}
	for i, s := range segs {
		dir := i < len(segs)-1 || isDir
		if dir && s == ".git" {
			return "another version control's folder"
		}
	}
	base := strings.ToLower(segs[len(segs)-1])
	if !isDir {
		switch base {
		case "desktop.ini", "thumbs.db", ".ds_store":
			return "system file"
		}
		if strings.HasPrefix(base, ".dawgit-") {
			return "DAWGit's temporary file"
		}
	}
	return ""
}

// Kind groups a file in the app (set, audio, …; "other" when no preset says).
func (p *Profile) Kind(rel string) string {
	if a, sub := p.presetFor(rel); a != nil && a.preset != nil {
		names := make([]string, 0, len(a.preset.Kinds))
		for k := range a.preset.Kinds {
			names = append(names, k)
		}
		sort.Strings(names)
		segs := strings.Split(strings.ToLower(sub), "/")
		for _, k := range names {
			for _, pat := range a.preset.Kinds[k] {
				// "*.png" (the usual kind) by its ending: the app asks for
				// every file of the project.
				if end, ok := endsWith(pat); ok {
					for _, s := range segs {
						if strings.HasSuffix(s, end) {
							return k
						}
					}
				} else if matchPath(pat, sub, false) {
					return k
				}
			}
		}
	}
	return "other"
}

// endsWith returns the lowercased ending a pattern like "*.png" matches
// (what matchPath does for it: any name in the path ending so).
func endsWith(pat string) (string, bool) {
	pat = strings.ToLower(strings.TrimSpace(pat))
	end, ok := strings.CutPrefix(pat, "*")
	if !ok || end == "" || strings.ContainsAny(end, `*?[\/`) {
		return "", false
	}
	return end, true
}

// Handler is the handler for a file (zero when none applies).
func (p *Profile) Handler(rel string) Handler {
	if a, sub := p.presetFor(rel); a != nil && a.preset != nil {
		for _, h := range a.preset.Handlers {
			for _, pat := range h.Files {
				if matchPath(pat, sub, false) {
					return h
				}
			}
		}
	}
	return Handler{}
}

// Running lists the running-tool checks of the presets in use (e.g.
// "ableton-live").
func (p *Profile) Running() []string {
	var out []string
	for _, a := range p.applied {
		if a.preset != nil && a.preset.Running != "" {
			out = append(out, a.preset.Running)
		}
	}
	return out
}

// Tool is the program of the project's main preset ("" when none says).
func (p *Profile) Tool() string {
	for i := len(p.applied) - 1; i >= 0; i-- { // the project folder's first
		if a := p.applied[i]; a.preset != nil && a.preset.Tool != "" {
			return a.preset.Tool
		}
	}
	return ""
}

// Openable lists what "Open in <tool>" offers in root: paths relative to it
// ("." for a preset's folder), and for each the opener ("" for the file's own
// program).
func (p *Profile) Openable(root string) (paths []string, openers map[string]string) {
	openers = map[string]string{}
	for _, a := range p.applied {
		if a.preset == nil {
			continue
		}
		for _, pat := range a.preset.Open.Files {
			if pat == "." {
				rel := a.Folder
				if rel == "" {
					rel = "."
				}
				paths = append(paths, rel)
				openers[rel] = a.preset.Open.With
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(a.Folder), filepath.FromSlash(pat)))
			for _, m := range matches {
				if rel, err := filepath.Rel(root, m); err == nil {
					rel = filepath.ToSlash(rel)
					paths = append(paths, rel)
					openers[rel] = a.preset.Open.With
				}
			}
		}
	}
	sort.Strings(paths)
	return paths, openers
}

// Checks lists the checks the presets in use run before a commit.
func (p *Profile) Checks() []string {
	var out []string
	for _, a := range p.applied {
		if a.preset != nil {
			out = append(out, a.preset.Checks...)
		}
	}
	return out
}

// NeedsNewer reports the version the profile requires when it is newer than
// current ("" when current will do).
func (p *Profile) NeedsNewer(current string) string {
	if p.Requires == "" {
		return ""
	}
	want, err1 := parseVersion(p.Requires)
	have, err2 := parseVersion(current)
	if err1 != nil || err2 != nil {
		return ""
	}
	for i := 0; i < 3; i++ {
		if want[i] != have[i] {
			if want[i] > have[i] {
				return p.Requires
			}
			return ""
		}
	}
	return ""
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, errors.New("bad version")
	}
	for i, s := range parts {
		n, err := strconv.Atoi(strings.SplitN(s, "-", 2)[0])
		if err != nil || n < 0 {
			return out, errors.New("bad version")
		}
		out[i] = n
	}
	return out, nil
}

// --- patterns ---
//
// Patterns work like .gitignore lines: "*.asd" (no slash) matches a name at
// any depth; "Exports/" matches folders only; a pattern with a slash
// ("Samples/*.wav", "/Backup/") is relative to the project (or the preset's
// folder); "**" matches any number of folders. Matching ignores case.
// A pattern matching a folder also matches everything inside it.

func checkPattern(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.New("empty pattern")
	}
	for _, seg := range strings.Split(strings.Trim(p, "/"), "/") {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("bad pattern %q", p)
		}
	}
	return nil
}

func matchPath(pattern, rel string, isDir bool) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	rel = strings.ToLower(rel)
	dirOnly := strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	anchored := strings.Contains(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	pat := strings.Split(pattern, "/")
	segs := strings.Split(rel, "/")
	// rel itself, then each folder above it.
	for n := len(segs); n >= 1; n-- {
		dir := n < len(segs) || isDir
		if dirOnly && !dir {
			continue
		}
		cand := segs[:n]
		if anchored {
			if globSegs(pat, cand) {
				return true
			}
		} else if ok, _ := path.Match(pat[0], cand[n-1]); ok {
			return true
		}
	}
	return false
}

// globSegs matches path segments against pattern segments with "**".
func globSegs(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globSegs(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, _ := path.Match(pat[0], segs[0]); !ok {
		return false
	}
	return globSegs(pat[1:], segs[1:])
}
