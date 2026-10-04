package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func TestBackup(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, err := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	h := sum("kick")
	if err := b.PutObject(h, bytes.NewReader([]byte("kick"))); err != nil {
		t.Fatal(err)
	}
	b.PutProject(remote.Project{ID: strings.Repeat("1", 32), Name: "Song"})
	pid := strings.Repeat("1", 32)
	v1, v2 := sum(`{"version":1}`), sum(`{"version":2}`)
	if err := b.PutSnapshot(pid, v1, []byte(`{"version":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateBranch(pid, "main", "", v1); err != nil {
		t.Fatal(err)
	}
	b.PutBackupStatus(strings.Repeat("9", 32), remote.BackupStatus{Kind: "folder"})

	dst := t.TempDir()
	rep, err := Run(b, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Copied == 0 || rep.Copied != rep.Keys {
		t.Fatalf("first run %+v", rep)
	}
	got, err := os.ReadFile(filepath.Join(dst, "objects", h[:2], h[2:]))
	if err != nil || string(got) != "kick" {
		t.Fatalf("object %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "backups")); err == nil {
		t.Error("members' backup records are not backed up")
	}
	if _, err := os.Stat(filepath.Join(dst, "README.txt")); err != nil {
		t.Error("no README")
	}

	// Again: nothing new, nothing copied.
	time.Sleep(1100 * time.Millisecond) // runs are named by the second
	rep, err = Run(b, dst, nil)
	if err != nil || rep.Copied != 0 {
		t.Fatalf("second run %+v %v", rep, err)
	}
	// The team moves on and cleans up: new things come, nothing goes.
	h2 := sum("snare")
	b.PutObject(h2, bytes.NewReader([]byte("snare")))
	b.PutSnapshot(pid, v2, []byte(`{"version":2}`))
	b.UpdateBranch(pid, "main", v1, v2)
	fake.Delete("band", "team/objects/"+h[:2]+"/"+h[2:])
	time.Sleep(1100 * time.Millisecond)
	rep, err = Run(b, dst, nil)
	if err != nil || rep.Copied != 3 {
		t.Fatalf("third run %+v %v", rep, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "objects", h[:2], h[2:])); err != nil {
		t.Error("a backup never deletes")
	}
	branch, _ := os.ReadFile(filepath.Join(dst, "projects", pid, "branches", "main"))
	if !strings.Contains(string(branch), v2) {
		t.Errorf("branch %q", branch)
	}
	runs, _ := Runs(dst)
	if len(runs) != 3 {
		t.Fatalf("runs %v", runs)
	}
	first, _ := os.ReadFile(filepath.Join(dst, "runs", runs[2]+".json"))
	if !strings.Contains(string(first), v1) {
		t.Errorf("the first run records the first version: %s", first)
	}
}

func TestLocalPaths(t *testing.T) {
	for _, bad := range []string{"", "../x", "a/../../x", "/abs", "C:/x", `a\b`, "a//b"} {
		if local("D", bad) != "" {
			t.Errorf("%q accepted", bad)
		}
	}
	if local("D", "objects/ab/cd") == "" {
		t.Error("plain key refused")
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestClaim(t *testing.T) {
	dir := t.TempDir()
	if err := Claim(filepath.Join(dir, "new"), "t1", "Band"); err != nil {
		t.Fatal(err)
	}
	if err := Claim(filepath.Join(dir, "new"), "t1", "Band"); err != nil {
		t.Error("its own folder again:", err)
	}
	if err := Claim(filepath.Join(dir, "new"), "t2", "Other"); err != ErrOtherTeam {
		t.Error("another team's:", err)
	}
	os.WriteFile(filepath.Join(dir, "song.als"), nil, 0o644)
	if err := Claim(dir, "t1", "Band"); err != ErrNotEmpty {
		t.Error("a folder with things in it:", err)
	}
	if Claimed(filepath.Join(dir, "new"), "t1") != nil || Claimed(dir, "t1") == nil {
		t.Error("Claimed")
	}
}
