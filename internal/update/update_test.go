package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/release"
)

const releases = `[
  {"tag_name": "v0.4.0", "html_url": "https://github.com/n0neye/DAWGit/releases/tag/v0.4.0", "draft": true, "assets": []},
  {"tag_name": "v0.3.10", "html_url": "https://github.com/n0neye/DAWGit/releases/tag/v0.3.10", "prerelease": true,
   "assets": [{"name": "DAWGit-0.3.10-setup.exe", "browser_download_url": "https://github.com/n0neye/DAWGit/releases/download/v0.3.10/DAWGit-0.3.10-setup.exe"}]},
  {"tag_name": "v0.3.9", "html_url": "https://github.com/n0neye/DAWGit/releases/tag/v0.3.9", "assets": []},
  {"tag_name": "v0.3.1", "html_url": "https://github.com/n0neye/DAWGit/releases/tag/v0.3.1", "assets": []},
  {"tag_name": "v9.9.9", "html_url": "https://example.com/elsewhere", "assets": []},
  {"tag_name": "nightly", "html_url": "https://github.com/n0neye/DAWGit/releases/tag/nightly", "assets": []}
]`

func serve(t *testing.T) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(releases))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestNewer(t *testing.T) {
	url := serve(t)
	r, err := Newer(url, "0.3.2")
	if err != nil {
		t.Fatal(err)
	}
	// Drafts, foreign links and odd tags are skipped; 0.3.10 > 0.3.9 numerically.
	if r == nil || r.Version != "0.3.10" ||
		r.DownloadURL != "https://github.com/n0neye/DAWGit/releases/download/v0.3.10/DAWGit-0.3.10-setup.exe" {
		t.Fatalf("newer = %+v", r)
	}
	if r, err := Newer(url, "0.3.10"); err != nil || r != nil {
		t.Fatalf("up to date: %+v %v", r, err)
	}
}

func TestParse(t *testing.T) {
	for s, want := range map[string]bool{"0.3.2": true, "v1.2.3": true, "1.2.3-dev": true, "1.2": false, "x.1.2": false} {
		if _, ok := parse(s); ok != want {
			t.Errorf("parse(%q) ok = %v", s, ok)
		}
	}
}

func TestFromFeed(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	site := srv.URL + "/"

	body = `{"version": "0.8.0", "download": "` + site + `DAWGit Pro-0.8.0-setup.exe", "notes": "` + site + `notes.html"}`
	r, err := FromFeed(srv.URL+"/latest.json", "0.7.0")
	if err != nil || r == nil || r.Version != "0.8.0" || r.DownloadURL != site+"DAWGit Pro-0.8.0-setup.exe" || r.PageURL != site+"notes.html" {
		t.Fatalf("newer: %+v %v", r, err)
	}
	if r, err := FromFeed(srv.URL+"/latest.json", "0.8.0"); err != nil || r != nil {
		t.Fatalf("same version: %+v %v", r, err)
	}
	// Links elsewhere are not handed out.
	body = `{"version": "0.9.0", "download": "https://elsewhere.example/x.exe"}`
	if r, _ := FromFeed(srv.URL+"/latest.json", "0.7.0"); r == nil || r.DownloadURL != "" || r.PageURL != "" {
		t.Fatalf("foreign link: %+v", r)
	}
	body = `not json`
	if _, err := FromFeed(srv.URL+"/latest.json", "0.7.0"); err == nil {
		t.Fatal("bad feed accepted")
	}
}

// A signed feed: the installer is downloaded and checked; a release naming a
// newer minimum is required.
func TestSignedUpdate(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "signing.key")
	pub, _ := release.NewKey(keyPath)
	key, _ := release.LoadKey(keyPath)
	old := PublicKey
	PublicKey = pub
	defer func() { PublicKey = old }()

	installer := []byte("MZ the installer")
	sum := sha256.Sum256(installer)
	m := release.Manifest{Version: "0.9.1", SHA256: hex.EncodeToString(sum[:]), MinVersion: "0.9.0"}
	release.Sign(key, &m)
	serveBytes := installer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".json") {
			json.NewEncoder(w).Encode(map[string]string{"version": m.Version, "download": "http://" + r.Host + "/DAWGit-0.9.1-setup.exe",
				"sha256": m.SHA256, "minVersion": m.MinVersion, "signature": m.Signature})
			return
		}
		w.Write(serveBytes)
	}))
	defer srv.Close()

	r, err := FromFeed(srv.URL+"/latest.json", "0.8.23")
	if err != nil || !r.Installable() || !r.Requires("0.8.23") || r.Requires("0.9.0") {
		t.Fatalf("release: %+v %v", r, err)
	}
	dir := t.TempDir()
	path, err := Download(r, dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(installer) {
		t.Fatalf("downloaded %q", got)
	}
	// Another installer than the signed one is refused (and not left there).
	os.Remove(path)
	serveBytes = []byte("MZ something else")
	if _, err := Download(r, dir, nil); err == nil {
		t.Fatal("a changed installer was accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
	// Signed with another key: offered as a page only.
	PublicKey = old
	if r, _ := FromFeed(srv.URL+"/latest.json", "0.8.23"); r.Installable() || r.Requires("0.8.23") {
		t.Fatalf("unsigned release: %+v", r)
	}
}

func TestNightlyOrder(t *testing.T) {
	for _, c := range []struct {
		a, b string
		less bool
	}{
		{"0.13.0-nightly.202610041530", "0.13.0", true},
		{"0.13.0", "0.13.0-nightly.202610041530", false},
		{"0.12.3", "0.13.0-nightly.202610041530", true},
		{"0.13.0-nightly.202610041530", "0.13.0-nightly.202610051200", true},
		{"0.13.0-nightly.202610051200", "0.13.0-nightly.202610041530", false},
		{"0.13.0", "0.13.0", false},
	} {
		if Older(c.a, c.b) != c.less {
			t.Errorf("Older(%s, %s) = %v", c.a, c.b, !c.less)
		}
	}
}

func TestSwitchTo(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
	defer srv.Close()
	body = `{"version": "0.12.3-nightly.202610041530", "download": "` + srv.URL + `/x.exe"}`
	// Not newer than Stable 0.12.3, but it is the other channel.
	if r, _ := FromFeed(srv.URL+"/nightly.json", "0.12.3"); r != nil {
		t.Fatalf("a plain check doesn't go back: %+v", r)
	}
	if r, err := SwitchTo(srv.URL+"/nightly.json", "0.12.3"); err != nil || r == nil || r.Version != "0.12.3-nightly.202610041530" {
		t.Fatalf("switch: %+v %v", r, err)
	}
	if r, _ := SwitchTo(srv.URL+"/nightly.json", "0.12.3-nightly.202610041530"); r != nil {
		t.Fatalf("already on it: %+v", r)
	}
}
