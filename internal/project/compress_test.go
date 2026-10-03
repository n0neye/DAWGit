package project

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/blob"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func stored(t *testing.T, fake *s3test.Server, content []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(content)
	h := hex.EncodeToString(sum[:])
	data, ok := fake.Object("team", "dawgit/objects/"+h[:2]+"/"+h[2:])
	if !ok {
		t.Fatalf("object %s not in storage", h[:8])
	}
	return data
}

// Files that compress go up compressed; others as they are. Teammates get
// the files back exactly.
func TestCompressedStorage(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	scene := strings.Repeat("--- !u!1 &123456\nGameObject:\n  m_Name: Cube\n", 5000)
	noise := make([]byte, 200<<10)
	rand.Read(noise)
	tricky := append([]byte(blob.Magic+"z"), []byte(strings.Repeat("a", 4000))...)
	write(t, a.Root, "Main.unity", scene)
	write(t, a.Root, "Stems/noise.wav", string(noise))
	write(t, a.Root, "tricky.bin", string(tricky))
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if s := stored(t, fake, []byte(scene)); !bytes.HasPrefix(s, []byte(blob.Magic+"z")) || len(s) > len(scene)/5 {
		t.Errorf("scene stored as %d bytes of %d", len(s), len(scene))
	}
	if s := stored(t, fake, noise); !bytes.Equal(s, noise) {
		t.Error("noise should be stored as it is")
	}
	if s := stored(t, fake, tricky); bytes.Equal(s, tricky) {
		t.Error("a file starting like a blob must be stored behind the header")
	}
	if left, _ := filepath.Glob(filepath.Join(a.Dir, "objects", "tmp", "blob-*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}

	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	for name, want := range map[string][]byte{"Main.unity": []byte(scene), "Stems/noise.wav": noise, "tricky.bin": tricky} {
		got, _ := os.ReadFile(filepath.Join(b.Root, filepath.FromSlash(name)))
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs after clone", name)
		}
	}
}
