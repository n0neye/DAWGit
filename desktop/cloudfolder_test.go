package desktop

import (
	"path/filepath"
	"testing"
)

func TestCloudFolder(t *testing.T) {
	od := filepath.Join(t.TempDir(), "SomeSyncedPlace")
	t.Setenv("OneDrive", od)
	t.Setenv("OneDriveConsumer", "")
	t.Setenv("OneDriveCommercial", "")
	for path, want := range map[string]string{
		filepath.Join(od, "Music", "Song Project"):                        "OneDrive",
		filepath.Join(od+"2", "Song Project"):                             "",
		filepath.FromSlash("C:/Users/me/Dropbox/Songs/Song Project"):      "Dropbox",
		filepath.FromSlash("C:/Users/me/Dropbox (Studio)/Song Project"):   "Dropbox",
		filepath.FromSlash("G:/My Drive/Song Project"):                    "Google Drive",
		filepath.FromSlash("C:/Users/me/OneDrive - Contoso/Song Project"): "OneDrive",
		filepath.FromSlash("D:/Music/Song Project"):                       "",
		filepath.FromSlash("D:/Music/Dropbox Stuff/Song Project"):         "",
	} {
		if got := cloudFolder(path); got != want {
			t.Errorf("%s: %q, want %q", path, got, want)
		}
	}
}
