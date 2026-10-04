package profile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsNightly(t *testing.T) {
	if _, ok := builtin["unity"]; ok {
		t.Skip("this build knows Unity (nightly presets registered)")
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "Assets"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ProjectSettings"), 0o755)
	if k := NightlyKind(dir); k != "Unity" {
		t.Errorf("kind %q", k)
	}
	os.WriteFile(filepath.Join(dir, FileName), []byte("presets:\n  .: unity\n"), 0o644)
	_, err := Load(dir)
	var nn *NeedsNightly
	if !errors.As(err, &nn) || nn.Kind != "Unity" {
		t.Errorf("load: %v", err)
	}
}
