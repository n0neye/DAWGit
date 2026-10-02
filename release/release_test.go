package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing.key")
	pub, err := NewKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewKey(path); err == nil {
		t.Error("a key is never replaced")
	}
	key, err := LoadKey(path)
	if err != nil || PublicKey(key) != pub {
		t.Fatalf("load: %v", err)
	}
	installer := filepath.Join(t.TempDir(), "setup.exe")
	os.WriteFile(installer, []byte("installer"), 0o644)
	sum, _ := FileSHA256(installer)
	m := Manifest{Version: "0.9.1", SHA256: sum, MinVersion: "0.9.0"}
	Sign(key, &m)
	if err := Verify(pub, m); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]Manifest{
		"another installer":  {Version: m.Version, SHA256: "00" + sum[2:], MinVersion: m.MinVersion, Signature: m.Signature},
		"another version":    {Version: "0.9.2", SHA256: sum, MinVersion: m.MinVersion, Signature: m.Signature},
		"no longer required": {Version: m.Version, SHA256: sum, Signature: m.Signature},
	} {
		if Verify(pub, bad) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if Verify("", m) == nil {
		t.Error("a build without a key accepts nothing")
	}
}
