package remote_test

import (
	"errors"
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
