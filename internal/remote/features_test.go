package remote_test

import (
	"errors"
	"strings"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

func TestTeamFeatures(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, err := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	if err != nil {
		t.Fatal(err)
	}
	url := fake.URL + "/band/team"
	if err := remote.Rename(b, "Band"); err != nil {
		t.Fatal(err)
	}
	if err := remote.CheckFeatures(b, url); err != nil {
		t.Fatal("no features:", err)
	}
	if err := remote.EnableFeature(b, "test-locks"); err != nil {
		t.Fatal(err)
	}
	// A rename keeps the features.
	remote.Rename(b, "Band 2")
	info, _ := b.Info()
	if info.Name != "Band 2" || len(info.Features) != 1 {
		t.Fatalf("info %+v", info)
	}
	var ef *remote.ErrTeamFeatures
	if err := remote.CheckFeatures(b, url+"?other"); !errors.As(err, &ef) || ef.Missing[0] != "test-locks" {
		t.Fatalf("unknown feature: %v", err)
	}
	remote.RegisterFeature("test-locks")
	if err := remote.CheckFeatures(b, url+"?again"); err != nil {
		t.Fatal("known now:", err)
	}
}

// Records rewritten by a build keep what a newer one added.
func TestRecordsKeepUnknownFields(t *testing.T) {
	fake := s3test.New("band")
	defer fake.Close()
	b, _ := remote.NewS3(fake.URL, "band", "team", "auto", "k", "s")
	pid, mid := strings.Repeat("1", 32), strings.Repeat("2", 32)
	fake.Put("band", "team/team.json", []byte(`{"name":"Band","future":{"x":1}}`))
	fake.Put("band", "team/projects/"+pid+"/project.json", []byte(`{"id":"`+pid+`","name":"Song","color":"red"}`))
	fake.Put("band", "team/members/"+mid+".json", []byte(`{"id":"`+mid+`","name":"Yi","avatar":"a.png"}`))
	if err := remote.Rename(b, "Band 2"); err != nil {
		t.Fatal(err)
	}
	if err := remote.RenameProject(b, pid, "Song 2"); err != nil {
		t.Fatal(err)
	}
	if err := remote.RenameMember(b, mid, "Yi 2"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"team/team.json": `"future"`,
		"team/projects/" + pid + "/project.json": `"color"`, "team/members/" + mid + ".json": `"avatar"`} {
		if got := object(fake, key); !strings.Contains(got, want) {
			t.Errorf("%s lost %s: %s", key, want, got)
		}
	}
	// A team.json that doesn't read is an error, and a rename doesn't overwrite it.
	fake.Put("band", "team/team.json", []byte(`{"name": broken`))
	if err := remote.Rename(b, "Band 3"); err == nil {
		t.Error("renamed over an unreadable team.json")
	}
}

func object(fake *s3test.Server, key string) string {
	b, _ := fake.Object("band", key)
	return string(b)
}
