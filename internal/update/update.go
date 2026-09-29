// Package update finds a newer DAWGit release on GitHub.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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
	for _, r := range list {
		v, ok := parse(r.Tag)
		if r.Draft || !ok || !less(bestV, v) || !strings.HasPrefix(r.URL, PageURLPrefix) {
			continue
		}
		rel := &Release{Version: strings.TrimPrefix(r.Tag, "v"), PageURL: r.URL}
		for _, a := range r.Assets {
			if strings.HasSuffix(strings.ToLower(a.Name), "-setup.exe") && strings.HasPrefix(a.URL, PageURLPrefix) {
				rel.DownloadURL = a.URL
			}
		}
		best, bestV = rel, v
	}
	return best, nil
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
