package desktop

import (
	"os"
	"path/filepath"
	"strings"
)

// cloudFolder names the file-syncing service whose folder holds root
// ("" when none). Such a service syncs .dawgit too: two computers writing
// it at once can damage the history, and files kept "online only" aren't
// really on disk.
func cloudFolder(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	abs = strings.ToLower(filepath.Clean(abs)) + string(filepath.Separator)
	for _, env := range []string{"OneDrive", "OneDriveConsumer", "OneDriveCommercial"} {
		if d := os.Getenv(env); d != "" && strings.HasPrefix(abs, strings.ToLower(filepath.Clean(d))+string(filepath.Separator)) {
			return "OneDrive"
		}
	}
	for _, seg := range strings.Split(abs, string(filepath.Separator)) {
		switch {
		case seg == "dropbox" || strings.HasPrefix(seg, "dropbox ("):
			return "Dropbox"
		case seg == "google drive" || seg == "my drive":
			return "Google Drive"
		case seg == "iclouddrive" || seg == "icloud drive":
			return "iCloud Drive"
		case seg == "onedrive" || strings.HasPrefix(seg, "onedrive - "):
			return "OneDrive"
		}
	}
	return ""
}
