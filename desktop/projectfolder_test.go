package desktop

import (
	"path/filepath"
	"testing"
)

func TestProjectFolder(t *testing.T) {
	song := filepath.Join(t.TempDir(), "Song Project")
	for in, want := range map[string]string{
		song:                           song,
		filepath.Join(song, ".dawgit"): song,
		filepath.Join(song, ".dawgit", "objects"):                   song,
		filepath.Join(song, "Samples") + string(filepath.Separator): filepath.Join(song, "Samples"),
	} {
		if got := projectFolder(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
