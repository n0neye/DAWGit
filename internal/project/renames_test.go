package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func move(t *testing.T, root, from, to string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(to))), 0o755)
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(from)), filepath.Join(root, filepath.FromSlash(to))); err != nil {
		t.Fatal(err)
	}
}

const lyrics = "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\n"

func TestMovesInStatusAndHistory(t *testing.T) {
	root := newProject(t)
	r, _ := Init(root, "yi")
	write(t, root, "Samples/kick.wav", "RIFF kick")
	write(t, root, "Notes/lyrics.txt", lyrics)
	write(t, root, "Art/cover.bin", "\x00\x01 cover")
	first := mustSnapshot(t, r, "first")

	move(t, root, "Samples/kick.wav", "Drums/kick.wav")                                    // moved
	move(t, root, "Notes/lyrics.txt", "Words/lyrics.txt")                                  // moved
	write(t, root, "Words/lyrics.txt", strings.Replace(lyrics, "line 3", "line three", 1)) // and changed
	move(t, root, "Art/cover.bin", "Art/front.bin")                                        // binary, moved
	write(t, root, "Art/front.bin", "\x00\x02 cover v2")                                   // and changed: not paired
	changes, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Change{}
	for _, c := range changes {
		got[c.Path] = c
	}
	if c := got["Drums/kick.wav"]; c.Status != "renamed" || c.From != "Samples/kick.wav" || c.Edited {
		t.Errorf("kick: %+v", c)
	}
	if c := got["Words/lyrics.txt"]; c.Status != "renamed" || c.From != "Notes/lyrics.txt" || !c.Edited {
		t.Errorf("lyrics: %+v", c)
	}
	if _, listed := got["Samples/kick.wav"]; listed {
		t.Error("a move is one change")
	}
	if got["Art/front.bin"].Status != "added" || got["Art/cover.bin"].Status != "deleted" {
		t.Errorf("binary changed while moved: %+v %+v", got["Art/front.bin"], got["Art/cover.bin"])
	}
	files, _ := r.Files(false)
	for _, f := range files {
		if f.Path == "Drums/kick.wav" && (f.Status != "renamed" || f.From != "Samples/kick.wav") {
			t.Errorf("listed: %+v", f)
		}
	}

	// Committing the move: both paths go together.
	r.Only = []string{"Drums/kick.wav", "Samples/kick.wav"}
	second := mustSnapshot(t, r, "move the kick")
	r.Only = nil
	if fm := second.FileMap(); fm["Drums/kick.wav"].Hash == "" || fm["Samples/kick.wav"].Hash != "" || fm["Notes/lyrics.txt"].Hash == "" {
		t.Fatalf("committed: %+v", second.Files)
	}
	vc, err := r.VersionChanges(second.ID)
	if err != nil || len(vc) != 1 || vc[0].Status != "renamed" || vc[0].From != "Samples/kick.wav" {
		t.Fatalf("version changes: %+v %v", vc, err)
	}
	mustSnapshot(t, r, "the rest")

	// The file's history goes back through the move.
	hist, err := r.FileHistory("Words/lyrics.txt")
	if err != nil || len(hist) != 2 {
		t.Fatalf("history: %+v %v", hist, err)
	}
	if hist[0].Status != "renamed" || hist[0].From != "Notes/lyrics.txt" || hist[1].Status != "added" ||
		hist[1].Path != "Notes/lyrics.txt" || hist[1].Version.ID != first.ID {
		t.Fatalf("history: %+v", hist)
	}
	// An old version comes back under today's name.
	if err := r.RestoreFileFrom("Words/lyrics.txt", "Notes/lyrics.txt", first.ID); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "Words", "lyrics.txt")); string(b) != lyrics {
		t.Fatalf("restored: %q", b)
	}
}

// One person moves a file, another changes it where it was: the change
// follows the file.
func TestChangesFollowAMove(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "Notes/lyrics.txt", lyrics)
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	move(t, a.Root, "Notes/lyrics.txt", "Words/lyrics.txt")
	if _, _, err := a.Save("tidy up", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(lyrics, "line 7", "line seven", 1)
	write(t, b.Root, "Notes/lyrics.txt", edited)
	_, res, err := b.Save("better line 7", Strategy("fail"))
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Words", "lyrics.txt")); string(got) != edited {
		t.Fatalf("the change should be in the moved file: %q (%+v)", got, res)
	}
	if _, err := os.Stat(filepath.Join(b.Root, "Notes", "lyrics.txt")); !os.IsNotExist(err) {
		t.Fatal("the old place should be gone")
	}
	found := false
	for _, l := range res.MergeLog {
		found = found || strings.Contains(l, "follow")
	}
	if !found {
		t.Errorf("merge log: %v", res.MergeLog)
	}
}
