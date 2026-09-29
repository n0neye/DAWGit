package project

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dawgit/internal/als"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
	"dawgit/internal/server"
	"dawgit/internal/teams"
)

const token = "test-token"

func newServer(t *testing.T) string {
	t.Helper()
	st, err := server.OpenStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Handler(st, token))
	t.Cleanup(srv.Close)
	return srv.URL
}

// team sets up machine A (who created the project) and machine B (a clone).
func team(t *testing.T) (a, b *Repo) {
	t.Helper()
	url := newServer(t)
	root := newProject(t)
	a, err := Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	a.SetRemote(url, token)
	if _, res, err := a.Save("v2", Strategy("fail")); err != nil || res.Action != "published" {
		t.Fatalf("first save: %v %+v", err, res)
	}
	dirB := filepath.Join(t.TempDir(), "B", "Song Project")
	b, m, err := Clone(url, token, "Song", dirB, "alex")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || m.ID != a.Head() {
		t.Fatalf("clone got %v, want %s", m, a.Head())
	}
	assertClean(t, b)
	return a, b
}

func setTracks(t *testing.T, r *Repo) map[string]als.Track {
	t.Helper()
	s, err := als.Load(filepath.Join(r.Root, "Song.als"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]als.Track{}
	for _, tr := range s.Tracks() {
		out[tr.Name()] = tr
	}
	return out
}

func TestTeamSaveMergesAndUpdateFastForwards(t *testing.T) {
	a, b := team(t)

	// A groups the audio tracks, B adds a drum track: made by hand in Live.
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, res, err := a.Save("group audio", Strategy("fail")); err != nil || res.Action != "published" {
		t.Fatalf("A save: %v %+v", err, res)
	}
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))

	// Both changed the bounce track: the default refuses and explains.
	_, _, err := b.Save("drums", Strategy("fail"))
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 ||
		!strings.Contains(conflict.Conflicts[0].String(), "Bounce + Reverb") {
		t.Fatalf("expected one track conflict, got %v", err)
	}
	// Nothing was published or written.
	if tr := setTracks(t, b); tr["Audios"].Elem != nil {
		t.Fatal("working set changed after a refused merge")
	}

	_, res, err := b.Save("drums", Strategy("both"))
	if err != nil || res.Action != "published" || len(res.MergeLog) == 0 {
		t.Fatalf("B save: %v %+v", err, res)
	}
	tracks := setTracks(t, b)
	// B is "ours" here: its bounce track stays, A's version is kept as a copy.
	for _, name := range []string{"Audios", "Drum", "5 Bounce + Reverb", "# Bounce + Reverb [theirs]"} {
		if tracks[name].Elem == nil {
			t.Errorf("merged set lacks %q (has %v)", name, keys(tracks))
		}
	}
	assertClean(t, b)
	m, _ := b.Load(b.Head())
	if len(m.Parents) != 2 {
		t.Errorf("merge version should have two parents: %v", m.Parents)
	}

	// A takes the merge: a fast forward.
	up, err := a.Update(Strategy("fail"))
	if err != nil || up.Action != "fast-forward" || up.To != b.Head() {
		t.Fatalf("A update: %v %+v", err, up)
	}
	if setTracks(t, a)["Drum"].Elem == nil {
		t.Error("A did not receive the drum track")
	}
	assertClean(t, a)
	// The relinked set still counts as that version: no empty version.
	if _, err := a.Snapshot("nothing"); !errors.Is(err, ErrNothingToSnapshot) {
		t.Errorf("expected nothing to snapshot after update, got %v", err)
	}
	if up, _ := a.Update(Strategy("fail")); up.Action != "up-to-date" {
		t.Errorf("second update: %+v", up)
	}
	// Everyone's versions are listed, merge first.
	log, _ := a.Log()
	var msgs []string
	for _, m := range log {
		msgs = append(msgs, m.Message)
	}
	if len(msgs) != 4 || msgs[0] != "Merge versions from the team" || msgs[3] != "v2" {
		t.Errorf("log = %v", msgs)
	}
}

func TestUpdateRefusesUnsavedChanges(t *testing.T) {
	a, b := team(t)
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	a.Save("group audio", Strategy("fail"))
	os.WriteFile(filepath.Join(b.Root, "Samples", "idea.wav"), []byte("RIFF"), 0o644)
	if _, err := b.Update(Strategy("fail")); !errors.Is(err, ErrDirty) {
		t.Fatalf("expected ErrDirty, got %v", err)
	}
}

func TestSampleChangedOnBothSides(t *testing.T) {
	a, b := team(t)
	os.WriteFile(filepath.Join(a.Root, "Samples", "vox.wav"), []byte("RIFF-base"), 0o644)
	a.Save("vox", Strategy("fail"))
	b.Update(Strategy("fail"))

	os.WriteFile(filepath.Join(a.Root, "Samples", "vox.wav"), []byte("RIFF-yi"), 0o644)
	a.Save("vox take 2", Strategy("fail"))
	os.WriteFile(filepath.Join(b.Root, "Samples", "vox.wav"), []byte("RIFF-alex"), 0o644)

	if _, _, err := b.Save("vox alex", Strategy("fail")); err == nil || !strings.Contains(err.Error(), "vox.wav") {
		t.Fatalf("expected a sample conflict, got %v", err)
	}
	if _, _, err := b.Save("vox alex", Strategy("both")); err != nil {
		t.Fatal(err)
	}
	mine, _ := os.ReadFile(filepath.Join(b.Root, "Samples", "vox.wav"))
	theirs, _ := os.ReadFile(filepath.Join(b.Root, "Samples", "vox (theirs).wav"))
	if string(mine) != "RIFF-alex" || string(theirs) != "RIFF-yi" {
		t.Errorf("mine=%q theirs=%q", mine, theirs)
	}
}

func TestServerRejectsBadToken(t *testing.T) {
	url := newServer(t)
	r, _ := Init(newProject(t), "yi")
	// Connecting checks the token right away.
	if err := r.SetRemote(url, "wrong"); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected token error, got %v", err)
	}
	if r.Config.Remote != nil {
		t.Error("project joined a team despite the bad token")
	}
}

// Credentials live in the per-user team store, never in the project folder;
// older project configs that still hold them are migrated.
func TestCredentialsStayOutOfProjectFolder(t *testing.T) {
	url := newServer(t)
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(url, token); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(a.Dir, "config.json"))
	if strings.Contains(string(data), token) {
		t.Fatalf("token written to the project folder: %s", data)
	}
	if _, _, err := a.Save("v1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	store, _ := teams.Load()
	tm := store.FindByURL(url)
	if tm == nil || tm.Remote.Token != token || store.ProjectRoot(tm.ID, a.Config.ProjectID) != a.Root {
		t.Fatalf("team store: %+v", store)
	}

	// An older project (0.1.x) with the token in its config.
	b, _ := Init(newProject(t), "alex")
	b.Config.Remote = &RemoteConfig{URL: url, Token: token}
	b.SaveConfig()
	if _, err := b.Client(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(b.Dir, "config.json"))
	if strings.Contains(string(data), token) {
		t.Fatalf("token not migrated out of the project folder: %s", data)
	}
	if _, _, err := b.Save("v1", Strategy("fail")); err != nil {
		t.Fatalf("migrated project cannot sync: %v", err)
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The GUI flow: try, get structured conflicts, decide each, try again.
func TestSaveWithPerConflictResolutions(t *testing.T) {
	a, b := team(t)
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	a.Save("group audio", Strategy("fail"))
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))

	_, _, err := b.Save("drums", MergeOptions{})
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %v", err)
	}
	c := conflict.Conflicts[0]
	if c.Key != "Song.als#track:14" || c.File != "Song.als" || !c.CanKeepBoth {
		t.Fatalf("conflict item: %+v", c)
	}
	_, res, err := b.Save("drums", MergeOptions{Resolutions: map[string]string{c.Key: "theirs"}})
	if err != nil || res.Action != "published" {
		t.Fatalf("save with resolution: %v %+v", err, res)
	}
	tracks := setTracks(t, b)
	if tracks["# Bounce + Reverb [theirs]"].Elem != nil {
		t.Error("theirs was chosen, no copy expected")
	}
	if tracks["Drum"].Elem == nil || tracks["Audios"].Elem == nil {
		t.Error("non-conflicting changes missing")
	}
}

// The same team workflow with no DAWGit server: an S3-compatible bucket,
// joined with a connection code.
func TestTeamOverObjectStorage(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})

	a, err := Init(newProject(t), "yi")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	if a.PollInterval() < 10*time.Second {
		t.Errorf("storage backends should poll slowly, got %v", a.PollInterval())
	}
	if _, res, err := a.Save("v2", Strategy("fail")); err != nil || res.Action != "published" {
		t.Fatalf("first save: %v %+v", err, res)
	}
	b, m, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil || m == nil || m.ID != a.Head() {
		t.Fatalf("clone: %v %v", err, m)
	}

	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, _, err := a.Save("group audio", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	if _, res, err := b.Save("drums", Strategy("both")); err != nil || res.Action != "published" {
		t.Fatalf("B save with merge: %v %+v", err, res)
	}
	if up, err := a.Update(Strategy("fail")); err != nil || up.Action != "fast-forward" {
		t.Fatalf("A update: %v %+v", err, up)
	}
	if setTracks(t, a)["Drum"].Elem == nil || setTracks(t, a)["Audios"].Elem == nil {
		t.Error("merged result incomplete")
	}
	// Soft locks work over storage too.
	copyFile(t, filepath.Join(fixtureProject, "SampleAbletonProject_v2.als"), filepath.Join(a.Root, "Song.als"))
	if _, err := a.ReportWorkspace(); err != nil {
		t.Fatal(err)
	}
	if mates, err := b.Teammates(); err != nil || len(mates) != 1 || mates[0].Author != "yi" {
		t.Fatalf("teammates over storage: %v %+v", err, mates)
	}
}

func TestSaveAndCloneReportProgress(t *testing.T) {
	url := newServer(t)
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(url, token); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	a.OnProgress = func(p Progress) {
		seen[p.Stage]++
		if p.Total > 0 && (p.Done < 0 || p.Done >= p.Total) {
			t.Errorf("progress out of range: %+v", p)
		}
	}
	sig := a.SetsSignature()
	if _, _, err := a.Save("v1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{StageScanning, StageStoring, StageUploading} {
		if seen[s] == 0 {
			t.Errorf("no %q progress while saving: %v", s, seen)
		}
	}
	if a.SetsSignature() == sig {
		t.Error("signature did not change with the new version")
	}

	downloads := 0
	tm, err := a.Team()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = CloneFromTeam(tm, a.Config.ProjectID, filepath.Join(t.TempDir(), "B"), "alex",
		func(p Progress) {
			if p.Stage == StageDownloading {
				downloads++
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if downloads == 0 {
		t.Error("no download progress while cloning")
	}
}
