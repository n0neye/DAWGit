package teams

import (
	"os"
	"path/filepath"
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
	// Same address: credentials updated, no duplicate, name kept.
	s.Upsert(remote.Config{URL: "http://studio.local:7331", Token: "t2"}, "Other")
	if len(s.Teams) != 1 || s.Teams[0].Remote.Token != "t2" || s.Teams[0].Name != "Studio Night" {
		t.Fatalf("teams = %+v", s.Teams)
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
