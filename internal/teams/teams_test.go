package teams

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// A backup bucket's keys are sealed too.
func TestBackupKeysSealed(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	s, _ := Load()
	tm := s.Upsert(remote.Config{URL: "s3+https://x.r2.cloudflarestorage.com/team/dawgit", AccessKey: "a", SecretKey: "b"}, "")
	tm.Backup = &Backup{Storage: &remote.Config{URL: "s3+https://y.example.com/vault/band", AccessKey: "AKIDBACKUP",
		SecretKey: "backupSecretK7MDENG"}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(Dir(), "teams.json"))
	if runtime.GOOS == "windows" && (strings.Contains(string(data), "backupSecretK7MDENG") || strings.Contains(string(data), "AKIDBACKUP")) {
		t.Fatalf("backup keys in the clear:\n%s", data)
	}
	if tm.Backup.Storage.SecretKey != "backupSecretK7MDENG" {
		t.Fatal("saving must not change the keys in memory")
	}
	back, err := Load()
	if err != nil || back.Teams[0].Backup.Storage.SecretKey != "backupSecretK7MDENG" ||
		back.Teams[0].Backup.Storage.AccessKey != "AKIDBACKUP" {
		t.Fatalf("read back: %+v %v", back.Teams[0].Backup, err)
	}
}

// DAWGit Pro's settings come over on a first start, on the Nightly channel.
func TestImportFromPro(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DAWGIT_CONFIG_DIR", filepath.Join(dir, "DAWGit"))
	pro := filepath.Join(dir, "DAWGit Pro", "teams.json")
	os.MkdirAll(filepath.Dir(pro), 0o700)
	os.WriteFile(pro, []byte(`{"author":"Yi","teams":[{"id":"t1","name":"Band","remote":{"url":"s3+https://x/b/f"}}],"projects":{}}`), 0o600)
	to := filepath.Join(Dir(), "teams.json")
	if ok, err := importFrom(pro, to); !ok || err != nil {
		t.Fatalf("import: %v %v", ok, err)
	}
	s, _ := Load()
	if s.Author != "Yi" || len(s.Teams) != 1 || s.Channel != "nightly" {
		t.Fatalf("imported: %+v", s)
	}
	if ok, _ := importFrom(pro, to); ok {
		t.Error("only on a first start")
	}
}

// teams.json is shared by Stable and Nightly: a save keeps what the other
// wrote, at every level.
func TestKeepsUnknownFields(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	os.MkdirAll(Dir(), 0o700)
	path := filepath.Join(Dir(), "teams.json")
	os.WriteFile(path, []byte(`{"author":"Yi","nightlyThing":1,
		"teams":[{"id":"t1","name":"Band","remote":{"url":"s3+https://x/b/f","broker":"y"},"labs":true,
			"backup":{"folder":"D:/B","cloud":"z"}}],"projects":{}}`), 0o600)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Teams[0].Name = "Band 2"
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{`"nightlyThing"`, `"labs"`, `"broker"`, `"cloud"`, `"Band 2"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("lost %s:\n%s", want, data)
		}
	}
}

// Changes made at the same time all land (the app, the command line tool
// and background work share teams.json).
func TestUpdateKeepsEveryChange(t *testing.T) {
	t.Setenv("DAWGIT_CONFIG_DIR", t.TempDir())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Update(func(s *Store) error {
				s.AddLocal(fmt.Sprintf("C:/p%d", i))
				return nil
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	s, _ := Load()
	if len(s.Local) != 20 {
		t.Fatalf("%d of 20 changes kept", len(s.Local))
	}
}
