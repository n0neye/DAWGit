package unity

import (
	"path/filepath"
	"strings"

	"dawgit/internal/wintitle"
)

// editorOpen reports "Unity Editor" when an editor window has this project
// open. Its title starts with the project folder's name: "URP_Sample -
// SampleScene - Windows, Mac, Linux - Unity 6000.0.59f2 <DX11>".
func editorOpen(root string) string {
	name := strings.ToLower(filepath.Base(root)) + " - "
	for _, t := range wintitle.Of("Unity.exe") {
		if t = strings.ToLower(t); strings.HasPrefix(t, name) && strings.Contains(t, " - unity ") {
			return "Unity Editor"
		}
	}
	return ""
}
