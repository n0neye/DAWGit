package teams

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"dawgit/internal/remote"
)

func TestStoreRoundTrip(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	s, err := Load()
	if err != nil || len(s.Teams) != 0 {
		t.Fatalf("empty store: %v %+v", err, s)
	}
	a := s.Upsert(remote.Config{URL: "HTTP://Studio.local:7331/", Token: "t1"}, "Studio Night")
	if a.Remote.URL != "http://studio.local:7331" || s.Current != a.ID {
		t.Fatalf("team = %+v, current %q", a, s.Current)
	}
	// Same address: credentials updated, no duplicate, the team's new name
	// taken, but not over a name the user gave.
	s.Upsert(remote.Config{URL: "http://studio.local:7331", Token: "t2"}, "Studio Nights")
	if len(s.Teams) != 1 || s.Teams[0].Remote.Token != "t2" || s.Teams[0].Name != "Studio Nights" {
		t.Fatalf("teams = %+v", s.Teams)
	}
	s.Rename(a.ID, "Our band")
	if s.SyncName(a.ID, "Studio Night") || a.Name != "Our band" {
		t.Fatalf("custom name overwritten: %+v", a)
	}
	s.Rename(a.ID, "")
	if !s.SyncName(a.ID, "Studio Night") || a.Name != "Studio Night" || a.CustomName {
		t.Fatalf("reset rename: %+v", a)
	}
	b := s.Upsert(remote.Config{URL: "s3+https://x.r2.cloudflarestorage.com/team/dawgit", AccessKey: "k", SecretKey: "s"}, "")
	if b.Name != "team" {
		t.Errorf("default storage name = %q", b.Name)
	}
	s.Teams[1].Name = "Storage team/dawgit" // the default before 0.2: renamed on load
	s.SetProjectRoot(a.ID, "p1", `C:\Music\Song Project`)
	s.AddLocal(`C:\Music\Solo Project`)
	s.Author = "yi"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(Dir(), "teams.json")); fi == nil {
		t.Fatal("not saved")
	}

	s2, _ := Load()
	if s2.Teams[1].Name != "team" {
		t.Errorf("old default name not updated: %q", s2.Teams[1].Name)
	}
	if s2.FindByURL("http://STUDIO.local:7331/") == nil || s2.ProjectRoot(a.ID, "p1") != `C:\Music\Song Project` ||
		s2.Author != "yi" || len(s2.Roots()) != 2 {
		t.Fatalf("reloaded = %+v", s2)
	}
	s2.Remove(a.ID)
	if s2.Current != b.ID || s2.ProjectRoot(a.ID, "p1") != "" || len(s2.Teams) != 1 {
		t.Fatalf("after remove = %+v", s2)
	}
}

// Keys are sealed in teams.json and read back; plain ones from older files
// still work.
func TestSecretsSealed(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	s, _ := Load()
	s.Upsert(remote.Config{URL: "s3+https://x.r2.cloudflarestorage.com/team/dawgit", AccessKey: "AKIDEXAMPLE",
		SecretKey: "wJalrXUtnFEMIsecretK7MDENG"}, "")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(Dir(), "teams.json"))
	if runtime.GOOS == "windows" && (strings.Contains(string(data), "secretK7MDENG") || strings.Contains(string(data), "AKIDEXAMPLE")) {
		t.Fatalf("keys in the clear:\n%s", data)
	}
	if s.Teams[0].Remote.SecretKey != "wJalrXUtnFEMIsecretK7MDENG" {
		t.Fatal("saving must not change the keys in memory")
	}
	back, err := Load()
	if err != nil || back.Teams[0].Remote.SecretKey != "wJalrXUtnFEMIsecretK7MDENG" ||
		back.Teams[0].Remote.AccessKey != "AKIDEXAMPLE" || back.Teams[0].KeysUnreadable {
		t.Fatalf("read back: %+v %v", back.Teams[0], err)
	}
	// Sealed somewhere else: the keys are gone, and the team says so.
	broken := strings.Replace(string(data), "dpapi:", "dpapi:AAAA", 1)
	os.WriteFile(filepath.Join(Dir(), "teams.json"), []byte(broken), 0o600)
	if runtime.GOOS == "windows" {
		if b, err := Load(); err != nil || !b.Teams[0].KeysUnreadable {
			t.Fatalf("unreadable keys: %+v %v", b, err)
		}
	}
}
