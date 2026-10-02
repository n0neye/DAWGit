package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"dawgit/internal/als"
	"dawgit/internal/audio"
	"dawgit/internal/convert"
	"dawgit/internal/preview"
	"dawgit/internal/project"
	"dawgit/internal/teams"
	"dawgit/internal/textdiff"
)

// ProjectFile is a file as the Changes tab lists it.
type ProjectFile struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added | modified | deleted | unchanged | ignored
	Size   int64  `json:"size"`
	Kind   string `json:"kind"` // set | live (clip, preset, rack) | audio | midi | other
	// Live is the Live that last saved a set, e.g. "Ableton Live 12.3.1".
	Live string `json:"live"`
	// Renamed: where it was, and whether its content changed too.
	From   string `json:"from"`
	Edited bool   `json:"edited"`
	// Preview: the app can show it as an image (/dawgit-preview); Video:
	// it can try to play it; Model: a 3D model it can show.
	Preview bool `json:"preview"`
	Video   bool `json:"video"`
	Model   bool `json:"model"`
}

// fileKind groups a file in the app (set, audio, …), as the project's rules
// say (the Ableton preset's kinds for an Ableton project).
func fileKind(r *project.Repo, p string) string {
	rules, _ := r.Profile()
	return rules.Kind(p)
}

// ProjectFiles lists the changed files; with all, every tracked file in the
// project folder (not those the rules leave out).
func (a *App) ProjectFiles(root string, all bool) ([]ProjectFile, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	files, err := r.Files(all)
	if err != nil {
		return nil, err
	}
	out := []ProjectFile{}
	for _, f := range files {
		pf := ProjectFile{Path: f.Path, Status: f.Status, Size: f.Size, Kind: fileKind(r, f.Path), From: f.From, Edited: f.Edited,
			Preview: preview.Supported(f.Path), Video: preview.IsVideo(f.Path), Model: preview.IsModel(f.Path)}
		if pf.Kind == "set" && f.Status != "deleted" {
			pf.Live = als.CreatorOf(r.Abs(f.Path))
		}
		out = append(out, pf)
	}
	return out, nil
}

// FileVersion is a version that changed a file.
type FileVersion struct {
	Version Version `json:"version"`
	Status  string  `json:"status"` // added | modified | deleted | renamed
	Path    string  `json:"path"`   // where the file is in that version
	From    string  `json:"from"`   // renamed: where it was before
}

// FileHistory lists the versions of the current branch that changed path.
func (a *App) FileHistory(root, file string) ([]FileVersion, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	hist, err := r.FileHistory(file)
	if err != nil {
		return nil, err
	}
	names := a.memberNames(r)
	out := []FileVersion{}
	for _, h := range hist {
		v := toVersion(h.Version, nil)
		if n := names[v.AuthorID]; n != "" {
			v.Author = n
		}
		out = append(out, FileVersion{Version: v, Status: h.Status, Path: h.Path, From: h.From})
	}
	return out, nil
}

// FileDiff describes how a set changed between two versions ("" for from:
// the set was added), as diff lines.
func (a *App) FileDiff(root, file, from, to string) ([]string, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	d, err := r.FileDiff(file, from, to)
	if err != nil {
		return nil, err
	}
	return diffLines(d), nil
}

// TextChanges is how a text file changed, line by line.
type TextChanges struct {
	Text      bool            `json:"text"`      // false: not text, or too big to compare
	TooBig    bool            `json:"tooBig"`    // over maxTextDiff
	Added     int             `json:"added"`     // lines
	Removed   int             `json:"removed"`   // lines
	Hunks     []textdiff.Hunk `json:"hunks"`     // the changes with lines around them
	Truncated bool            `json:"truncated"` // more changes than shown
}

const (
	maxTextDiff  = 8 << 20 // bytes per side
	maxDiffLines = 5000    // lines sent to the view
	maxWhole     = 20000   // lines sent when showing a whole file
)

// readText reads file in a version (a version id, "" for the project folder
// now, "none" for no file: empty), at most maxTextDiff+1 bytes; a file known
// to be bigger isn't downloaded from the team just to find that out.
func readText(r *project.Repo, file, version string) ([]byte, error) {
	if version == "none" {
		return nil, nil
	}
	if version != "" {
		if id, err := r.Resolve(version); err == nil {
			if m, err := r.Load(id); err == nil {
				if f, ok := m.FileMap()[file]; ok && f.Size > maxTextDiff {
					return make([]byte, maxTextDiff+1), nil
				}
			}
		}
	}
	f, err := r.OpenFile(file, version)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxTextDiff+1))
}

// TextDiff compares a text file in two versions. A version is a version id,
// "" for the project folder now, or "none" when there is no file to compare
// with (it was added or deleted). whole: every line, not just the changes
// with a few around them.
// fromFile: the file's path in from, when it was elsewhere ("" for file).
func (a *App) TextDiff(root, file, from, to string, whole bool, fromFile string) (*TextChanges, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if fromFile == "" {
		fromFile = file
	}
	old, err := readText(r, fromFile, from)
	if err != nil {
		return nil, err
	}
	cur, err := readText(r, file, to)
	if err != nil {
		return nil, err
	}
	out := &TextChanges{Hunks: []textdiff.Hunk{}}
	if len(old) > maxTextDiff || len(cur) > maxTextDiff {
		out.TooBig = true
		return out, nil
	}
	if !isText(old) || !isText(cur) {
		return out, nil
	}
	out.Text = true
	a1, b1 := textdiff.Split(string(old)), textdiff.Split(string(cur))
	context, limit := 3, maxDiffLines
	if whole {
		context, limit = len(a1)+len(b1), maxWhole
	}
	shown := 0
	for _, h := range textdiff.Hunks(a1, b1, context) {
		for _, l := range h.Lines {
			switch l.Kind {
			case "add":
				out.Added++
			case "del":
				out.Removed++
			}
		}
		if shown+len(h.Lines) > limit && shown > 0 {
			out.Truncated = true
			continue
		}
		if len(h.Lines) > limit {
			h.Lines = h.Lines[:limit]
			out.Truncated = true
		}
		shown += len(h.Lines)
		out.Hunks = append(out.Hunks, h)
	}
	return out, nil
}

// TextContent is a text file's lines (TextFile).
type TextContent struct {
	Text      bool     `json:"text"`   // false: not text, too big, or no file
	TooBig    bool     `json:"tooBig"` // over maxTextDiff
	Lines     []string `json:"lines"`
	Truncated bool     `json:"truncated"` // more lines than sent
}

// TextFile reads a text file in a version ("" for the project folder now).
func (a *App) TextFile(root, file, version string) (*TextContent, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	data, err := readText(r, file, version)
	if err != nil {
		return nil, err
	}
	out := &TextContent{Lines: []string{}}
	switch {
	case len(data) > maxTextDiff:
		out.TooBig = true
	case isText(data) && len(data) > 0:
		out.Text = true
		out.Lines = textdiff.Split(string(data))
		if len(out.Lines) > maxWhole {
			out.Lines, out.Truncated = out.Lines[:maxWhole], true
		}
	case len(data) == 0:
		out.Text = true // an empty file
	}
	return out, nil
}

// isText: UTF-8 without NUL bytes (a UTF-8 BOM is fine).
func isText(b []byte) bool {
	return bytes.IndexByte(b, 0) < 0 && utf8.Valid(b)
}

// DiscardFile puts one file back as it is in the version the project is on;
// from: where a moved file was (it goes back there).
func (a *App) DiscardFile(root, file, from string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if fileKind(r, file) == "set" {
		if set := liveGuard(r, force); set != "" {
			return blocked(set), nil
		}
	}
	if err := r.RestoreFile(file, ""); err != nil {
		return nil, err
	}
	if from != "" { // a move: back where it was too
		if err := r.RestoreFile(from, ""); err != nil {
			return nil, err
		}
	}
	return &Result{Action: "discarded", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
}

// DiscardFiles puts some changed files back as they are in the version the
// project is on (a moved file goes back where it was too).
func (a *App) DiscardFiles(root string, files []string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	from := map[string]string{}
	for _, c := range changes {
		from[c.Path] = c.From
	}
	for _, f := range files {
		if fileKind(r, f) == "set" {
			if set := liveGuard(r, force); set != "" {
				return blocked(set), nil
			}
		}
	}
	for _, f := range files {
		if err := r.RestoreFile(f, ""); err != nil {
			return nil, err
		}
		if from[f] != "" {
			if err := r.RestoreFile(from[f], ""); err != nil {
				return nil, err
			}
		}
	}
	return &Result{Action: "discarded", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
}

// RestoreFileVersion puts one file back as it was in a version; the rest of
// the project stays. The result is an uncommitted change. source: the file's
// path in that version, when it had another ("" for file).
func (a *App) RestoreFileVersion(root, file, version, source string, force bool) (*Result, error) {
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	if version == "" {
		return nil, errors.New("pick a version")
	}
	if fileKind(r, file) == "set" {
		if set := liveGuard(r, force); set != "" {
			return blocked(set), nil
		}
	}
	if source == "" {
		source = file
	}
	if err := r.RestoreFileFrom(file, source, version); err != nil {
		return nil, err
	}
	return &Result{Action: "restored", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
}

// DiscardAll drops every uncommitted change: the project folder goes back to
// the version it is on.
func (a *App) DiscardAll(root string, force bool) (*Result, error) {
	defer a.tidyLater(root)
	r, unlock, err := a.open(root)
	if err != nil {
		return nil, err
	}
	defer unlock()
	changes, err := r.Status()
	if err != nil {
		return nil, err
	}
	for _, c := range changes { // only rewriting a set needs Live to let go of it
		if fileKind(r, c.Path) == "set" {
			if set := liveGuard(r, force); set != "" {
				return blocked(set), nil
			}
			break
		}
	}
	head := r.Head()
	if head == "" {
		return nil, errors.New("no version yet: nothing to go back to")
	}
	if _, _, err := r.Checkout(head, true); err != nil {
		return nil, err
	}
	return &Result{Action: "discarded", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
}

// ShowFile opens Explorer with the file selected.
func (a *App) ShowFile(root, file string) error {
	if !safeRel(file) {
		return errors.New("invalid path")
	}
	return shellSelect(filepath.Join(root, filepath.FromSlash(file)))
}

func safeRel(p string) bool {
	return p != "" && !filepath.IsAbs(p) && !strings.Contains(filepath.ToSlash(p), "..")
}

// fileServer serves a project file (now, or as in a version) to the web
// view, for audio previews:
//
//	/dawgit-file?root=<project folder>&path=<relative path>&version=<id or "">
//
// Only folders of known projects are served. AIFF becomes WAV, which the web
// view can play.
func (a *App) fileServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/dawgit-peaks" {
			a.servePeaks(w, req)
			return
		}
		if req.URL.Path == "/dawgit-preview" {
			a.servePreview(w, req)
			return
		}
		if req.URL.Path != "/dawgit-file" {
			next.ServeHTTP(w, req)
			return
		}
		q := req.URL.Query()
		root, rel, version := q.Get("root"), q.Get("path"), q.Get("version")
		if !safeRel(rel) || !knownProject(root) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		r, err := project.Open(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		f, err := r.OpenFile(rel, version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		defer f.Close()
		if version == "" {
			w.Header().Set("Cache-Control", "no-store") // the file on disk changes
		} else {
			w.Header().Set("Cache-Control", "max-age=31536000, immutable") // a version never does
		}
		name := path.Base(rel)
		ext := strings.ToLower(path.Ext(name))
		if ext == ".aif" || ext == ".aiff" {
			data, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			wav, err := audio.AIFFToWAV(data)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
				return
			}
			w.Header().Set("Content-Type", "audio/wav")
			http.ServeContent(w, req, name+".wav", time.Time{}, bytes.NewReader(wav))
			return
		}
		t := mime.TypeByExtension(ext)
		if ext == ".mov" || ext == ".m4v" {
			t = "video/mp4" // the web view plays these when it knows the codec, not by their own types
		}
		if t != "" {
			w.Header().Set("Content-Type", t)
		}
		http.ServeContent(w, req, name, time.Time{}, f)
	})
}

// peaksCache keeps waveforms of files in versions (they never change).
var peaksCache = struct {
	sync.Mutex
	m map[string]*audio.Waveform
}{m: map[string]*audio.Waveform{}}

// servePeaks answers /dawgit-peaks (same parameters as /dawgit-file, plus
// n slices) with a waveform overview as JSON, for WAV and AIFF. Other
// formats get 415: the page decodes those itself.
func (a *App) servePeaks(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	root, rel, version := q.Get("root"), q.Get("path"), q.Get("version")
	n, _ := strconv.Atoi(q.Get("n"))
	if n < 50 || n > 4000 {
		n = 800
	}
	if !safeRel(rel) || !knownProject(root) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ext := strings.ToLower(path.Ext(rel))
	if ext != ".wav" && ext != ".aif" && ext != ".aiff" {
		http.Error(w, "decode it in the page", http.StatusUnsupportedMediaType)
		return
	}
	key := fmt.Sprintf("%s|%s|%s|%d", root, rel, version, n)
	peaksCache.Lock()
	wf := peaksCache.m[key]
	peaksCache.Unlock()
	if wf == nil {
		r, err := project.Open(root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		f, err := r.OpenFile(rel, version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if ext == ".wav" {
			wf, err = audio.WAVPeaks(f, n)
		} else {
			var data []byte
			if data, err = io.ReadAll(f); err == nil {
				wf, err = audio.AIFFPeaks(data, n)
			}
		}
		f.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
		if version != "" {
			peaksCache.Lock()
			if len(peaksCache.m) > 500 {
				peaksCache.m = map[string]*audio.Waveform{}
			}
			peaksCache.m[key] = wf
			peaksCache.Unlock()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(wf)
}

// previewCache keeps recent previews (decoding a big PSD takes a moment), by
// file content: a version's never changes; the folder's is keyed by size
// and time.
var previewCache = struct {
	sync.Mutex
	m     map[string]cachedPreview
	bytes int
}{m: map[string]cachedPreview{}}

type cachedPreview struct {
	data []byte
	typ  string
}

const previewCacheBytes = 200 << 20

// servePreview answers /dawgit-preview (same parameters as /dawgit-file,
// plus max: the longest side) with an image of a design file: Photoshop,
// TIFF, TGA, Affinity, Blender, Cinema 4D, or images the page shows as
// they are. 415 for files without a preview.
func (a *App) servePreview(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	root, rel, version := q.Get("root"), q.Get("path"), q.Get("version")
	maxSide, _ := strconv.Atoi(q.Get("max"))
	if maxSide <= 0 || maxSide > 8192 {
		maxSide = 2048
	}
	if !safeRel(rel) || !knownProject(root) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !preview.Supported(rel) {
		http.Error(w, "no preview", http.StatusUnsupportedMediaType)
		return
	}
	r, err := project.Open(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	key := fmt.Sprintf("%s|%d|", version, maxSide)
	if version == "" {
		fi, err := os.Stat(r.Abs(rel))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		key += fmt.Sprintf("%s|%s|%d|%d", root, rel, fi.Size(), fi.ModTime().UnixNano())
	} else {
		key += root + "|" + rel
	}
	previewCache.Lock()
	c, ok := previewCache.m[key]
	previewCache.Unlock()
	if !ok {
		f, err := r.OpenFile(rel, version)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		c.data, c.typ, err = preview.Image(rel, f, maxSide)
		f.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
			return
		}
		previewCache.Lock()
		if previewCache.bytes+len(c.data) > previewCacheBytes {
			previewCache.m, previewCache.bytes = map[string]cachedPreview{}, 0
		}
		previewCache.m[key] = c
		previewCache.bytes += len(c.data)
		previewCache.Unlock()
	}
	w.Header().Set("Content-Type", c.typ)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(c.data)
}

// knownProject reports whether root is a project folder the app lists.
func knownProject(root string) bool {
	store, err := teams.Load()
	if err != nil {
		return false
	}
	for _, r := range append(store.Roots(), store.Local...) {
		if strings.EqualFold(filepath.Clean(r), filepath.Clean(root)) {
			return true
		}
	}
	return false
}

// ConvertFormat is a format the Convert dialog offers.
type ConvertFormat struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ext      string `json:"ext"`
	Bitrates []int  `json:"bitrates"` // kbps, first is the default; none for lossless
	Rates    []int  `json:"rates"`    // sample rates it takes (Hz)
	Bits     []int  `json:"bits"`     // bit depths to choose from (FLAC)
}

func (a *App) ConvertFormats() []ConvertFormat {
	out := []ConvertFormat{}
	for _, f := range convert.Formats {
		out = append(out, ConvertFormat{ID: f.ID, Name: f.Name, Ext: f.Ext, Bitrates: append([]int{}, f.Bitrate...),
			Rates: append([]int{}, f.Rates...), Bits: append([]int{}, f.Bits...)})
	}
	return out
}

// ConvertInfo describes a sample (rate, channels, bits, length).
func (a *App) ConvertInfo(root, file string) (convert.Info, error) {
	if !safeRel(file) {
		return convert.Info{}, errors.New("invalid path")
	}
	return convert.Probe(filepath.Join(root, filepath.FromSlash(file)))
}

// ConvertPlan tells what a conversion would write, with notes on what the
// format forces. kbps: 0 for the default; rate, channels, bits: 0 keeps the
// original's.
func (a *App) ConvertPlan(root, file, format string, kbps, rate, channels, bits int) (convert.Result, error) {
	in, err := a.ConvertInfo(root, file)
	if err != nil {
		return convert.Result{}, err
	}
	return convert.Plan(format, in, convert.Options{Bitrate: kbps, Rate: rate, Channels: channels, Bits: bits})
}

// ConvertTarget is the file a conversion would write (relative path).
func (a *App) ConvertTarget(root, file, format string) (string, error) {
	if !safeRel(file) {
		return "", errors.New("invalid path")
	}
	dst, err := convert.Target(filepath.Join(root, filepath.FromSlash(file)), format)
	if err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(root, dst)
	return filepath.ToSlash(rel), nil
}

// ConvertFile writes a sample in another format next to it and returns the
// new file's path. Progress comes as "progress" events (stage "converting").
func (a *App) ConvertFile(root, file, format string, kbps, rate, channels, bits int) (string, error) {
	if !safeRel(file) || !knownProject(root) {
		return "", errors.New("invalid path")
	}
	src := filepath.Join(root, filepath.FromSlash(file))
	dst, err := convert.Target(src, format)
	if err != nil {
		return "", err
	}
	report, done := a.progressFor(root)
	defer done()
	err = convert.Convert(src, dst, convert.Options{Format: format, Bitrate: kbps, Rate: rate, Channels: channels, Bits: bits,
		Progress: func(p float64) {
			report(project.Progress{Stage: "converting", Done: int(p * 1000), Total: 1000})
		}})
	if err != nil {
		return "", err
	}
	rel, _ := filepath.Rel(root, dst)
	return filepath.ToSlash(rel), nil
}
