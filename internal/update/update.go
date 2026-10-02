// Package update finds a newer DAWGit release (on GitHub, or an extension
// build's feed), downloads its installer and checks it is the one the
// publisher signed (see dawgit/release).
package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dawgit/release"
)

// PublicKey checks the signatures of releases (made with dawgit-release).
// Builds with extensions are signed with the same key.
var PublicKey = "yiPaQTxcNM847ymdJcAp+gLj4DqNCEylPpI98sRrAQE="

// Repo is where releases are published.
const Repo = "n0neye/DAWGit"

// ReleasesAPI lists the repository's releases, previews included (the
// "latest" endpoint skips them).
var ReleasesAPI = "https://api.github.com/repos/" + Repo + "/releases?per_page=30"

// PageURLPrefix is the start of every link this package hands out.
const PageURLPrefix = "https://github.com/" + Repo + "/"

type Release struct {
	Version     string // e.g. "0.3.3"
	PageURL     string // the release page (what's new)
	DownloadURL string // the installer; "" if the release has none
	// Manifest: the installer's signed description; nil when the release
	// has none or its signature doesn't check (then it isn't installed by
	// DAWGit, only offered as a download).
	Manifest *release.Manifest
}

// Installable: DAWGit can download, check and install it.
func (r *Release) Installable() bool { return r != nil && r.Manifest != nil && r.DownloadURL != "" }

// Requires says whether current is too old to keep working: the release
// names a newer minimum.
func (r *Release) Requires(current string) bool {
	if r == nil || r.Manifest == nil || r.Manifest.MinVersion == "" {
		return false
	}
	return Older(current, r.Manifest.MinVersion)
}

// Older: version a is before b ("0.9.0" < "0.10.0").
func Older(a, b string) bool {
	va, ok1 := parse(a)
	vb, ok2 := parse(b)
	return ok1 && ok2 && less(va, vb)
}

// checked returns m when it is signed and describes version.
func checked(m release.Manifest, version string) *release.Manifest {
	if strings.TrimPrefix(m.Version, "v") != strings.TrimPrefix(version, "v") || release.Verify(PublicKey, m) != nil {
		return nil
	}
	return &m
}

type ghRelease struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Draft  bool   `json:"draft"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Newer returns the newest published release after current, or nil when
// current is up to date.
func Newer(apiURL, current string) (*Release, error) {
	cur, ok := parse(current)
	if !ok {
		return nil, fmt.Errorf("bad current version %q", current)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "DAWGit/"+current)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release check: %s", resp.Status)
	}
	var list []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}
	var best *Release
	bestV := cur
	bestManifest := ""
	for _, r := range list {
		v, ok := parse(r.Tag)
		if r.Draft || !ok || !less(bestV, v) || !strings.HasPrefix(r.URL, PageURLPrefix) {
			continue
		}
		rel := &Release{Version: strings.TrimPrefix(r.Tag, "v"), PageURL: r.URL}
		manifest := ""
		for _, a := range r.Assets {
			name := strings.ToLower(a.Name)
			if !strings.HasPrefix(a.URL, PageURLPrefix) {
				continue
			}
			if strings.HasSuffix(name, "-setup.exe") {
				rel.DownloadURL = a.URL
			} else if name == "update.json" {
				manifest = a.URL
			}
		}
		best, bestV = rel, v
		bestManifest = manifest
	}
	if best != nil && bestManifest != "" {
		var m release.Manifest
		if getJSON(client, bestManifest, current, &m) == nil {
			best.Manifest = checked(m, best.Version)
		}
	}
	return best, nil
}

func getJSON(client *http.Client, address, current string, out any) error {
	req, err := http.NewRequest("GET", address, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DAWGit/"+current)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", address, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(out)
}

// parse reads "1.2.3" or "v1.2.3" (anything after a "-" or "+" is ignored).
func parse(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	var v [3]int
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// Feed is an update feed's address for a build with extensions (set through
// ext.SetUpdateFeed): a JSON file {"version", "download", "notes"} next to
// the installer. "" checks this repository's GitHub releases.
var Feed string

type feedEntry struct {
	Version    string `json:"version"`  // e.g. "0.7.1"
	Download   string `json:"download"` // the installer
	Notes      string `json:"notes"`    // what's new (a page), optional
	SHA256     string `json:"sha256"`   // the installer's (signed, with the next two)
	MinVersion string `json:"minVersion"`
	Signature  string `json:"signature"`
}

// FromFeed returns the feed's release when it is newer than current.
// Its links must be on the feed's own site.
func FromFeed(feedURL, current string) (*Release, error) {
	cur, ok := parse(current)
	if !ok {
		return nil, fmt.Errorf("bad current version %q", current)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "DAWGit/"+current)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update feed: %s", resp.Status)
	}
	var e feedEntry
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return nil, fmt.Errorf("update feed: %w", err)
	}
	v, ok := parse(e.Version)
	if !ok || !less(cur, v) {
		return nil, nil
	}
	site := FeedSite(feedURL)
	rel := &Release{Version: strings.TrimPrefix(e.Version, "v")}
	if strings.HasPrefix(e.Download, site) {
		rel.DownloadURL = e.Download
	}
	if strings.HasPrefix(e.Notes, site) {
		rel.PageURL = e.Notes
	} else {
		rel.PageURL = rel.DownloadURL
	}
	if e.Signature != "" {
		rel.Manifest = checked(release.Manifest{Version: e.Version, SHA256: e.SHA256, MinVersion: e.MinVersion,
			Signature: e.Signature}, rel.Version)
	}
	return rel, nil
}

// Download fetches the release's installer into dir, checking it is the one
// the publisher signed, and returns its path. progress (may be nil) hears
// how many bytes of how many came.
func Download(r *Release, dir string, progress func(done, total int64)) (string, error) {
	if !r.Installable() {
		return "", errors.New("this update can't be installed by DAWGit: download it from its page")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := filepath.Base(strings.ReplaceAll(r.DownloadURL, "%20", " "))
	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
		name = "DAWGit-" + r.Version + "-setup.exe"
	}
	final := filepath.Join(dir, name)
	// Already here and whole (an earlier download): use it.
	if sum, err := release.FileSHA256(final); err == nil && strings.EqualFold(sum, r.Manifest.SHA256) {
		return final, nil
	}
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(r.DownloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading the update: %s", resp.Status)
	}
	tmp, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := tmp.Write(buf[:n]); err != nil {
				tmp.Close()
				return "", err
			}
			h.Write(buf[:n])
			done += int64(n)
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			tmp.Close()
			return "", rerr
		}
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if sum := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(sum, r.Manifest.SHA256) {
		return "", errors.New("the downloaded update isn't the one that was signed: not installed")
	}
	os.Remove(final)
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}

// FeedSite is the scheme and host of a feed ("https://example.com/"); its
// links must start with it.
func FeedSite(feedURL string) string {
	u, err := url.Parse(feedURL)
	if err != nil || u.Host == "" {
		return "\x00" // matches nothing
	}
	return u.Scheme + "://" + u.Host + "/"
}
