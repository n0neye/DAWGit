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
	if err := a.SetRemote(url, token); err != nil {
		t.Fatalf("connect: %v", err) // seen once, rarely: say why
	}
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
	// Taken in before committing: B's version comes after A's, no merge.
	m, _ := b.Load(b.Head())
	if len(m.Parents) != 1 || m.Parents[0] != a.Head() {
		t.Errorf("B's version should follow A's: %v", m.Parents)
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
	// Everyone's versions are listed, in one line.
	log, _ := a.Log()
	var msgs []string
	for _, m := range log {
		msgs = append(msgs, m.Message)
	}
	if len(msgs) != 3 || msgs[0] != "drums" || msgs[1] != "group audio" || msgs[2] != "v2" {
		t.Errorf("log = %v", msgs)
	}
}

// Getting the team's versions keeps uncommitted work: merged in, still
// uncommitted, and no version is made.
func TestUpdateKeepsUncommittedWork(t *testing.T) {
	a, b := team(t)
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	if _, _, err := a.Save("group audio", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	before, _ := b.Log()
	os.WriteFile(filepath.Join(b.Root, "Samples", "idea.wav"), []byte("RIFF idea"), 0o644)
	up, err := b.Update(Strategy("fail"))
	if err != nil || up.Action != "fast-forward" || !up.KeptWork || up.To != a.Head() || b.Head() != a.Head() {
		t.Fatalf("update: %v %+v", err, up)
	}
	if setTracks(t, b)["Audios"].Elem == nil {
		t.Error("A's set not taken in")
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Samples", "idea.wav")); string(got) != "RIFF idea" {
		t.Errorf("work lost: %q", got)
	}
	changes, _ := b.Status()
	if len(changes) != 1 || changes[0].Path != "Samples/idea.wav" || changes[0].Status != "added" {
		t.Errorf("still uncommitted: %+v", changes)
	}
	after, _ := b.Log()
	if len(after) != len(before)+1 { // A's version only
		t.Errorf("log %d -> %d versions", len(before), len(after))
	}

	// Committing it then is a version after A's: no merge.
	m, res, err := b.Save("idea", Strategy("fail"))
	if err != nil || res.Action != "published" || len(m.Parents) != 1 || m.Parents[0] != a.Head() {
		t.Fatalf("save: %v %+v %v", err, res, m)
	}

	// A crash halfway: the work comes back as it was.
	kept, _ := os.ReadFile(filepath.Join(b.Dir, keptWorkFile))
	os.Remove(filepath.Join(b.Root, "Samples", "idea.wav"))
	os.WriteFile(filepath.Join(b.Dir, switchingFile), []byte("work "+strings.Fields(string(kept))[0]+"\n"), 0o644)
	if b.UnfinishedSwitch() == "" {
		t.Fatal("unfinished switch not seen")
	}
	if _, err := b.RecoverSwitch(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Samples", "idea.wav")); string(got) != "RIFF idea" {
		t.Errorf("not recovered: %q", got)
	}
}

// Both changed the same set: merged track by track into the working set;
// where both changed one track, the update asks first.
func TestUpdateMergesUncommittedSet(t *testing.T) {
	a, b := team(t)
	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	a.Save("group audio", Strategy("fail"))
	copyFile(t, filepath.Join(fixtureProject, "Split-B.als"), filepath.Join(b.Root, "Song.als"))
	head := b.Head()
	_, err := b.Update(Strategy("fail"))
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) || len(conflict.Conflicts) != 1 {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if b.Head() != head || setTracks(t, b)["Audios"].Elem != nil || setTracks(t, b)["Drum"].Elem == nil {
		t.Fatal("a refused update changed something")
	}
	up, err := b.Update(Strategy("both"))
	if err != nil || !up.KeptWork || b.Head() != a.Head() {
		t.Fatalf("update: %v %+v", err, up)
	}
	tracks := setTracks(t, b)
	if tracks["Audios"].Elem == nil || tracks["Drum"].Elem == nil {
		t.Errorf("merged set lacks a side: %v", keys(tracks))
	}
	if changes, _ := b.Status(); len(changes) != 1 || changes[0].Path != "Song.als" {
		t.Errorf("uncommitted: %+v", changes)
	}
	// Tidying (as after every operation) keeps what the merged set needs:
	// committing it later works.
	b.PruneObjects()
	b.GC()
	m, res, err := b.Save("drums", Strategy("fail"))
	if err != nil || res.Action != "published" || len(m.Parents) != 1 || m.Parents[0] != a.Head() {
		t.Fatalf("save after tidying: %v %+v", err, res)
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
	// New versions are noticed over storage too.
	copyFile(t, filepath.Join(fixtureProject, "SampleAbletonProject_v2.als"), filepath.Join(a.Root, "Song.als"))
	if _, _, err := a.Save("back to v2", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if in, err := b.IncomingVersions(); err != nil || len(in) != 1 || in[0].Message != "back to v2" {
		t.Fatalf("incoming over storage: %v %+v", err, in)
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
		if p.Total > 0 && (p.Done < 0 || p.Done > p.Total) {
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

// A team member's versions record their id; renaming them in the team's
// member list renames all their versions.
func TestMemberIdentity(t *testing.T) {
	a, _ := team(t)
	store, _ := teams.Load()
	tm := store.FindByURL(a.Config.Remote.URL)
	tm.MemberID, tm.MemberName = teams.NewID(16), "Yi"
	store.Save()
	c, _ := a.Client()
	c.PutMember(remote.Member{ID: tm.MemberID, Name: "Yi"})

	copyFile(t, filepath.Join(fixtureProject, "Split-A.als"), filepath.Join(a.Root, "Song.als"))
	m, _, err := a.Save("with id", Strategy("fail"))
	if err != nil {
		t.Fatal(err)
	}
	if m.AuthorID != tm.MemberID || m.Author != "Yi" {
		t.Fatalf("author %q id %q", m.Author, m.AuthorID)
	}
	c.PutMember(remote.Member{ID: tm.MemberID, Name: "Yi Chen"})
	names := a.MemberNames()
	if got := AuthorName(m, names); got != "Yi Chen" {
		t.Errorf("renamed author = %q", got)
	}
	first, _ := a.Load(m.Parents[0]) // from before ids: keeps its recorded name
	if got := AuthorName(first, names); got != "yi" {
		t.Errorf("old version author = %q", got)
	}
}

// B's copy points the project's samples at B's folder (relinked): that is
// no change of B's, so A's change to such a track merges without a conflict.
func TestRelinkedTrackIsNotAChange(t *testing.T) {
	a, b := team(t)
	volume := func(r *Repo, track, value string) {
		t.Helper()
		p := filepath.Join(r.Root, "Song.als")
		s, err := als.Load(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, tr := range s.Tracks() {
			if tr.Name() == track {
				tr.Elem.Find("DeviceChain/Mixer/Volume/Manual").Set("Value", value)
			}
		}
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	volume(a, "5 Bounce + Reverb", "0.25")
	if _, _, err := a.Save("quieter bounce", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	volume(b, "3-Vital", "0.5")
	if _, _, err := b.Save("quieter synth", Strategy("fail")); err != nil {
		t.Fatalf("B save: %v", err)
	}
	tracks := setTracks(t, b)
	if v := tracks["3-Vital"].Elem.Val("DeviceChain/Mixer/Volume/Manual", ""); v != "0.5" {
		t.Errorf("B's change lost: volume %s", v)
	}
	if v := tracks["5 Bounce + Reverb"].Elem.Val("DeviceChain/Mixer/Volume/Manual", ""); v != "0.25" {
		t.Errorf("A's change not merged: volume %s", v)
	}
	s, _ := als.Load(filepath.Join(b.Root, "Song.als"))
	for _, ref := range s.SampleRefs() {
		if ref.RelativePathType == "3" && !strings.HasPrefix(ref.Path, filepath.ToSlash(b.Root)) {
			t.Errorf("B's sample points elsewhere: %s", ref.Path)
		}
	}
}
