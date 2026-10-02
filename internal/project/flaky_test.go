package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// Storage failing now and then (500 InternalError, as R2 does) doesn't
// fail a save or a download: the requests are made again.
func TestFlakyStorage(t *testing.T) {
	defer func(w time.Duration) { remote.RetryWait = w }(remote.RetryWait)
	remote.RetryWait = time.Millisecond
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		write(t, a.Root, filepath.Join("Stems", strings.Repeat("x", i+1)+".wav"), strings.Repeat("RIFF", i+1))
	}
	fake.Flaky = 7
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatalf("save: %v", err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, _ := os.ReadFile(filepath.Join(b.Root, "Stems", strings.Repeat("x", i+1)+".wav"))
		if string(got) != strings.Repeat("RIFF", i+1) {
			t.Fatalf("stem %d: %q", i, got)
		}
	}
}
