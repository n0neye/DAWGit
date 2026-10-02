package project

import (
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// The size of a team project is known before downloading it.
func TestSizeOnTeam(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, a.Root, "Notes/lyrics.txt", "twelve bytes")
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	head, _ := a.Load(a.Head())
	var want int64
	for _, f := range head.Files {
		want += f.Size
	}
	tm, err := Connect(code, "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := SizeOnTeam(tm, a.Config.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != len(head.Files) || got.Bytes != want || want == 0 {
		t.Errorf("size %+v, want %d files, %d bytes", got, len(head.Files), want)
	}
}
