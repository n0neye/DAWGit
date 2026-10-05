package project

import (
	"errors"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// A team that turned on a feature this build doesn't know can't be joined:
// the error says to update or switch to Nightly. (Working with a team
// already joined is checked in remote.CheckFeatures, through teams.Open.)
func TestJoinNeedsTeamFeatures(t *testing.T) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	if _, err := Connect(code, ""); err != nil {
		t.Fatal("no features:", err)
	}
	fake.Put("team", "dawgit/team.json", []byte(`{"name":"Band","features":["from-the-future"]}`))
	var tf *remote.ErrTeamFeatures
	if _, err := Connect(code, ""); !errors.As(err, &tf) || tf.Missing[0] != "from-the-future" {
		t.Fatalf("join: %v", err)
	}
}
