package unity

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// openEditor opens the project in the Unity version it was made with (from
// ProjectSettings/ProjectVersion.txt), as Unity Hub would.
func openEditor(root, rel string) error {
	v, err := editorVersion(root)
	if err != nil {
		return err
	}
	exe := findEditor(v)
	if exe == "" {
		return fmt.Errorf("Unity %s isn't installed: add it in Unity Hub (Installs)", v)
	}
	return exec.Command(exe, "-projectPath", root).Start()
}

func editorVersion(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "ProjectSettings", "ProjectVersion.txt"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "m_EditorVersion:"); ok {
			return strings.TrimSpace(v), nil
		}
	}
	return "", errors.New("ProjectVersion.txt doesn't name a Unity version")
}

// findEditor looks where Unity Hub installs editors.
func findEditor(version string) string {
	for _, d := range hubDirs() {
		exe := filepath.Join(d, version, "Editor", "Unity.exe")
		if _, err := os.Stat(exe); err == nil {
			return exe
		}
	}
	return ""
}

// hubDirs lists the folders Unity Hub installs editors into, one subfolder
// per version.
func hubDirs() []string {
	var dirs []string
	for _, env := range []string{"ProgramFiles", "ProgramW6432"} {
		if d := os.Getenv(env); d != "" {
			dirs = append(dirs, filepath.Join(d, "Unity", "Hub", "Editor"))
		}
	}
	// Unity Hub's "Installs location" when moved elsewhere: a JSON string.
	if data, err := os.ReadFile(filepath.Join(os.Getenv("APPDATA"), "UnityHub", "secondaryInstallPath.json")); err == nil {
		var d string
		if json.Unmarshal(data, &d) == nil && d != "" {
			dirs = append(dirs, filepath.FromSlash(d))
		}
	}
	return dirs
}
