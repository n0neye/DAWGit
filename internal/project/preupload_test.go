package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dawgit/internal/chunk"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func preuploadTeam(t *testing.T) (*Repo, string) {
	t.Helper()
	fake := s3test.New("team")
	t.Cleanup(fake.Close)
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	r, _ := Init(newProject(t), "yi")
	if err := r.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Save("v1", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	return r, code
}

// A big file that stopped changing goes up before the commit; the commit
// then neither copies it into the history nor uploads it.
func TestPreupload(t *testing.T) {
	r, code := preuploadTeam(t)
	defer func(m int64, s time.Duration) { PreuploadMin, PreuploadStable = m, s }(PreuploadMin, PreuploadStable)
	PreuploadMin, PreuploadStable = 1000, time.Minute
	video := strings.Repeat("frame ", 2000)
	write(t, r.Root, "Video/take.mov", video)
	write(t, r.Root, "notes.txt", "small")
	if c, _ := r.PreuploadCandidates(time.Now()); len(c) != 0 {
		t.Fatalf("still being written: %+v", c)
	}
	cands, err := r.PreuploadCandidates(time.Now().Add(2 * time.Minute))
	if err != nil || len(cands) != 1 || cands[0].Path != "Video/take.mov" {
		t.Fatalf("candidates: %+v %v", cands, err)
	}
	c, _ := r.Client()
	var done, total int64
	if err := r.Preupload(c, cands[0], func(d, tt int64) { done, total = d, tt }); err != nil {
		t.Fatal(err)
	}
	if done == 0 || done != total {
		t.Errorf("progress %d of %d", done, total)
	}
	if err := r.NotePreuploaded(cands[0].Hash); err != nil {
		t.Fatal(err)
	}
	if missing, _ := c.MissingObjects([]string{cands[0].Hash}); len(missing) != 0 {
		t.Fatal("not in the team's storage")
	}
	if entries, _ := os.ReadDir(filepath.Join(r.Dir, preuploadDir)); len(entries) != 0 {
		t.Errorf("copy left: %v", entries)
	}
	// Local cleanup before the commit keeps it listed.
	if _, err := r.GC(); err != nil {
		t.Fatal(err)
	}
	r.remote = nil
	if !r.remoteOnly()[cands[0].Hash] {
		t.Fatal("cleanup forgot the early upload")
	}
	if c, _ := r.PreuploadCandidates(time.Now().Add(2 * time.Minute)); len(c) != 0 {
		t.Errorf("again: %+v", c)
	}
	// The commit: not copied into the history, shared all the same.
	if _, _, err := r.Save("with the take", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if r.Store.Has(cands[0].Hash) {
		t.Error("the commit copied it into the history")
	}
	if got := cloneFiles(t, code, r.Config.Name); got["Video/take.mov"] != video {
		t.Errorf("a teammate gets %d bytes", len(got["Video/take.mov"]))
	}
}

// A file that changes between being found and copied doesn't go up under
// the hash it had.
func TestPreuploadChangedMeanwhile(t *testing.T) {
	r, _ := preuploadTeam(t)
	defer func(m int64, s time.Duration) { PreuploadMin, PreuploadStable = m, s }(PreuploadMin, PreuploadStable)
	PreuploadMin, PreuploadStable = 1000, time.Minute
	write(t, r.Root, "take.mov", strings.Repeat("a", 5000))
	cands, _ := r.PreuploadCandidates(time.Now().Add(2 * time.Minute))
	if len(cands) != 1 {
		t.Fatalf("candidates: %+v", cands)
	}
	write(t, r.Root, "take.mov", strings.Repeat("b", 5000))
	c, _ := r.Client()
	if err := r.Preupload(c, cands[0], nil); !errors.Is(err, ErrChangedSince) {
		t.Fatalf("preupload: %v", err)
	}
	if missing, _ := c.MissingObjects([]string{cands[0].Hash}); len(missing) != 1 {
		t.Fatal("went up under the old hash")
	}
}

// A file kept as pieces goes up as pieces, its list included.
func TestPreuploadChunked(t *testing.T) {
	r, code := preuploadTeam(t)
	defer func(m int64, s time.Duration) { PreuploadMin, PreuploadStable = m, s }(PreuploadMin, PreuploadStable)
	PreuploadMin, PreuploadStable = 1000, time.Minute
	big := make([]byte, chunk.MinFile+12345)
	for i := range big {
		big[i] = byte(i*7 + i/1000)
	}
	write(t, r.Root, "Video/long.mov", string(big))
	cands, err := r.PreuploadCandidates(time.Now().Add(2 * time.Minute))
	if err != nil || len(cands) != 1 {
		t.Fatalf("candidates: %+v %v", cands, err)
	}
	c, _ := r.Client()
	if err := r.Preupload(c, cands[0], nil); err != nil {
		t.Fatal(err)
	}
	r.NotePreuploaded(cands[0].Hash)
	if _, _, err := r.Save("long take", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if got := cloneFiles(t, code, r.Config.Name); got["Video/long.mov"] != string(big) {
		t.Errorf("a teammate gets %d bytes", len(got["Video/long.mov"]))
	}
}
