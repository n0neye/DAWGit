package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
