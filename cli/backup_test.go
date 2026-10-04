package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"dawgit/internal/project"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func TestBackupCommand(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/band/songs",
		AccessKey: "key", SecretKey: "secret"})
	a := newFolder(t, map[string]string{"Samples/kick.wav": "kick"})
	r, err := project.Init(a, "yi")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	if exit, rep := run(t, a, "save", "-m", "first"); exit != 0 {
		t.Fatalf("save: %+v", rep.Error)
	}

	if exit, rep := run(t, a, "backup", "run"); exit != 2 || rep.Error.Code != "usage" {
		t.Fatalf("no folder chosen in the app, none given: %+v", rep.Error)
	}
	full := t.TempDir()
	os.WriteFile(filepath.Join(full, "song.als"), nil, 0o644)
	if exit, rep := run(t, a, "backup", "run", full); exit != 1 || rep.Error.Code != "backup_folder_not_empty" {
		t.Fatalf("a folder with things in it: %+v", rep.Error)
	}

	dst := filepath.Join(t.TempDir(), "Backup")
	exit, rep := run(t, a, "backup", "run", dst)
	var out backupRunJSON
	json.Unmarshal(rep.Result, &out)
	if exit != 0 || out.Copied == 0 || out.Copied != out.Keys || out.Run == "" {
		t.Fatalf("backup run: %s %+v", rep.Result, rep.Error)
	}
	if _, err := os.Stat(filepath.Join(dst, "runs", out.Run+".json")); err != nil {
		t.Error("no record of the run:", err)
	}
	exit, rep = run(t, a, "backup", "run", dst)
	json.Unmarshal(rep.Result, &out)
	if exit != 0 || out.Copied != 0 {
		t.Fatalf("again: %s", rep.Result)
	}

	exit, rep = run(t, a, "backup", "status")
	var st backupStatusJSON
	json.Unmarshal(rep.Result, &st)
	if exit != 0 || !st.Supported || st.Folder != "" {
		t.Fatalf("status: %s", rep.Result)
	}
	if exit, rep := run(t, a, "backup", "status", "--team", "nobody"); exit != 1 {
		t.Fatalf("unknown team: %+v", rep)
	}
}
