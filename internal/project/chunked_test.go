package project

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dawgit/internal/blob"
	"dawgit/internal/chunk"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func randomBytes(seed int64, n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(seed)).Read(b)
	return b
}

func objectAt(fake *s3test.Server, h string) ([]byte, bool) {
	return fake.Object("team", "dawgit/objects/"+h[:2]+"/"+h[2:])
}

// A big file goes up as pieces; after an edit only the changed pieces go up,
// and a teammate downloads only those.
func TestChunkedStorage(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	level := randomBytes(1, 24<<20)
	small := randomBytes(2, 1<<20)
	write(t, a.Root, "Maps/Level.umap", string(level))
	write(t, a.Root, "Maps/Small.uasset", string(small))
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatalf("save: %v", err)
	}
	h := chunk.HashOf(level)
	if obj, ok := objectAt(fake, h); !ok || !blob.IsChunkList(obj) {
		t.Fatal("the level should be stored as a chunk list")
	}
	if keys := fake.Keys("team", "dawgit/chunked/"); len(keys) != 1 || !strings.HasSuffix(keys[0], h) {
		t.Fatalf("markers: %v", keys)
	}
	if obj, _ := objectAt(fake, chunk.HashOf(small)); !bytes.Equal(obj, small) {
		t.Error("small files stay whole")
	}

	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Maps", "Level.umap")); !bytes.Equal(got, level) {
		t.Fatal("level differs after clone")
	}

	// An actor added in the middle of the level.
	at := 11 << 20
	edited := append(append(append([]byte{}, level[:at]...), []byte("a new actor")...), level[at:]...)
	write(t, a.Root, "Maps/Level.umap", string(edited))
	put := fake.PutBytes
	if _, _, err := a.Save("an actor", Strategy("fail")); err != nil {
		t.Fatalf("save: %v", err)
	}
	if sent := fake.PutBytes - put; sent > 2*chunk.Max {
		t.Errorf("the edit sent %d bytes", sent)
	}
	get := fake.GetBytes
	if _, err := b.Update(Strategy("fail")); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Maps", "Level.umap")); !bytes.Equal(got, edited) {
		t.Fatal("level differs after update")
	}
	if recv := fake.GetBytes - get; recv > 2*chunk.Max {
		t.Errorf("the update downloaded %d bytes", recv)
	}

	// A piece lost from storage: verify tells, and puts it back.
	l := a.loadChunkList(chunk.HashOf(edited))
	if l == nil {
		t.Fatal("no chunk list kept here")
	}
	lost := l.Pieces[5].Hash
	fake.Delete("team", "dawgit/objects/"+lost[:2]+"/"+lost[2:])
	// (Both versions have that piece: the old one's level is not here, the
	// new one's mends both.)
	rep, err := a.Verify(false)
	if err != nil || len(rep.Problems) != 2 || !strings.Contains(rep.Problems[0].Detail, "lacks 1 of its") {
		t.Fatalf("verify: %+v %v", rep, err)
	}
	if rep, err = a.Verify(true); err != nil || len(rep.Problems) != 1 || !rep.Problems[0].Fixed {
		t.Fatalf("repair: %+v %v", rep, err)
	}
	if rep, err = a.Verify(false); err != nil || len(rep.Problems) != 0 {
		t.Fatalf("after repair: %+v %v", rep, err)
	}
	if _, ok := objectAt(fake, lost); !ok {
		t.Error("piece not uploaded again")
	}

	// A third computer, from scratch: all of it, right.
	c, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "C", "Song Project"), "sam")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(c.Root, "Maps", "Level.umap")); !bytes.Equal(got, edited) {
		t.Fatal("level differs on a new clone")
	}
}

// Storage failing now and then doesn't fail pieces going up or down.
func TestChunkedFlaky(t *testing.T) {
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
	level := append(randomBytes(3, 12<<20), bytes.Repeat([]byte("landscape "), 1<<20)...) // part compresses
	write(t, a.Root, "Maps/Level.umap", string(level))
	fake.Flaky = 5
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatalf("save: %v", err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(b.Root, "Maps", "Level.umap")); !bytes.Equal(got, level) {
		t.Fatal("level differs")
	}
}
