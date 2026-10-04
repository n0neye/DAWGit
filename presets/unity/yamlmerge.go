package unity

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// yamlMergeTimeout bounds one UnityYAMLMerge run: on input it can't read it
// shows an error dialog and waits, so it is ended instead.
var yamlMergeTimeout = 20 * time.Second

// findYAMLMerge returns the UnityYAMLMerge that comes with every Unity
// editor, from the newest installed version ("" when Unity isn't installed).
var findYAMLMerge = func() string {
	best, bestVer := "", ""
	for _, d := range hubDirs() {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			exe := filepath.Join(d, e.Name(), "Editor", "Data", "Tools", "UnityYAMLMerge.exe")
			if _, err := os.Stat(exe); err == nil && (best == "" || versionLess(bestVer, e.Name())) {
				best, bestVer = exe, e.Name()
			}
		}
	}
	return best
}

// versionLess compares Unity versions such as 2022.3.15f1 and 6000.0.40f1
// number by number.
func versionLess(a, b string) bool {
	na, nb := versionNumbers(a), versionNumbers(b)
	for i := 0; i < len(na) && i < len(nb); i++ {
		if na[i] != nb[i] {
			return na[i] < nb[i]
		}
	}
	return len(na) < len(nb)
}

func versionNumbers(v string) []int {
	var out []int
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		n, _ := strconv.Atoi(f)
		out = append(out, n)
	}
	return out
}

// mergeYAML merges scenes, prefabs and other assets Unity saves as text
// with UnityYAMLMerge, which knows their objects and properties: edits to
// different objects or properties combine even when they sit on nearby
// lines. When both sides changed the same property, or the file isn't Unity
// text (binary serialization), it reports a conflict and the person picks a
// whole version.
func mergeYAML(base, ours, theirs []byte) ([]byte, bool, error) {
	for _, b := range [][]byte{base, ours, theirs} {
		if !bytes.HasPrefix(b, []byte("%YAML")) {
			return nil, false, nil
		}
	}
	tool := findYAMLMerge()
	if tool == "" {
		return nil, false, nil
	}
	dir, err := os.MkdirTemp("", "dawgit-yamlmerge-")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)
	// Every file is named .unity: UnityYAMLMerge picks how to merge by the
	// extension and, for one it doesn't know (.mat, .asset, none), shows an
	// error dialog. Its scene merge reads any Unity text asset.
	files := map[string][]byte{"base.unity": base, "ours.unity": ours, "theirs.unity": theirs}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return nil, false, err
		}
	}
	out := filepath.Join(dir, "merged.unity")
	ctx, cancel := context.WithTimeout(context.Background(), yamlMergeTimeout)
	defer cancel()
	// Argument order as Unity documents it for git: base, theirs (remote),
	// ours (local), result. --fallback none: on a conflict, exit instead of
	// starting another merge tool.
	cmd := exec.CommandContext(ctx, tool, "merge", "-p", "--fallback", "none",
		filepath.Join(dir, "base.unity"), filepath.Join(dir, "theirs.unity"), filepath.Join(dir, "ours.unity"), out)
	hideWindow(cmd)
	// Any failure (a conflict, input it couldn't read, a timeout) leaves the
	// choice to the person, so the error itself isn't passed on.
	if cmd.Run() != nil {
		return nil, false, nil
	}
	merged, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(merged, []byte("%YAML")) {
		return nil, false, nil
	}
	return merged, true, nil
}
