package livecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessNamesIncludesSelf(t *testing.T) {
	names, err := processNames()
	if err != nil {
		t.Fatal(err)
	}
	self := strings.ToLower(filepath.Base(os.Args[0]))
	for _, n := range names {
		if strings.ToLower(filepath.Base(strings.TrimSpace(n))) == self {
			return
		}
	}
	t.Fatalf("own process %q not among %d processes", self, len(names))
}

func TestSetName(t *testing.T) {
	for title, want := range map[string]string{
		"Song - Ableton Live 12 Suite":                      "Song",
		"Song* - Ableton Live 12 Suite":                     "Song",
		"My Song [My Song Project] - Ableton Live 11 Suite": "My Song",
		"Split - A - Ableton Live 12 Standard":              "Split - A",
		"Ableton Live 12 Suite":                             "",
		"Preferences":                                       "",
	} {
		if got := SetName(title); got != want {
			t.Errorf("SetName(%q) = %q, want %q", title, got, want)
		}
	}
}

// With Live running, print what it has open (go test -run LiveProbe -v).
func TestLiveProbe(t *testing.T) {
	if !Running() {
		t.Skip("Ableton Live is not running")
	}
	titles, err := liveWindowTitles()
	t.Logf("titles %q (err %v)", titles, err)
	t.Logf("fixture project open set: %q", OpenSet(filepath.Join("..", "..", "testdata", "live")))
	t.Logf("repo root open set: %q", OpenSet(filepath.Join("..", "..")))
}
