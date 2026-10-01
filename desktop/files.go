package desktop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"dawgit/internal/als"
	"dawgit/internal/audio"
	"dawgit/internal/convert"
	"dawgit/internal/project"
	"dawgit/internal/teams"
)

// ProjectFile is a file as the Changes tab lists it.
type ProjectFile struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added | modified | deleted | unchanged | ignored
	Size   int64  `json:"size"`
	Kind   string `json:"kind"` // set | live (clip, preset, rack) | audio | midi | other
	// Live is the Live that last saved a set, e.g. "Ableton Live 12.3.1".
	Live string `json:"live"`
}

// fileKind groups a file in the app (set, audio, …), as the project's rules
// say (the Ableton preset's kinds for an Ableton project).
func fileKind(r *project.Repo, p string) string {
	rules, _ := r.Profile()
	return rules.Kind(p)
}

// ProjectFiles lists the changed files; with all, every file in the project
// folder (files DAWGit leaves out are "ignored").
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
		pf := ProjectFile{Path: f.Path, Status: f.Status, Size: f.Size, Kind: fileKind(r, f.Path)}
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
	Status  string  `json:"status"` // added | modified | deleted
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
		out = append(out, FileVersion{Version: v, Status: h.Status})
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

// DiscardFile puts one file back as it is in the version the project is on.
func (a *App) DiscardFile(root, file string, force bool) (*Result, error) {
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
	return &Result{Action: "discarded", Log: []string{}, Relinked: []string{}, Conflicts: []Conflict{}}, nil
}

// RestoreFileVersion puts one file back as it was in a version; the rest of
// the project stays. The result is an uncommitted change.
func (a *App) RestoreFileVersion(root, file, version string, force bool) (*Result, error) {
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
	if err := r.RestoreFile(file, version); err != nil {
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
		if t := mime.TypeByExtension(ext); t != "" {
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
