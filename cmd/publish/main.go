// Command publish uploads a built installer and its update feed to the
// releases bucket: the Nightly channel's (nightly.json), or another feed:
//
//	go run ./cmd/publish dist\DAWGit-0.13.0-nightly.202610041530-setup.exe 0.13.0-nightly.202610041530 [-min 0.9.7] [-feed nightly.json]
//
// The feed is signed with the release key (see dawgit-release), so DAWGit
// installs the update itself; -min makes older versions update before they
// go on. (Stable releases go to GitHub with update.json instead.)
//
// The bucket and its write key come from the environment (never stored in
// this repository): DAWGIT_RELEASE_ENDPOINT, DAWGIT_RELEASE_BUCKET,
// DAWGIT_RELEASE_ACCESS_KEY, DAWGIT_RELEASE_SECRET_KEY, and the bucket's
// public address DAWGIT_RELEASE_PUBLIC_URL (e.g. https://pub-….r2.dev).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/s3put"
	"dawgit/release"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "publish:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: publish <installer.exe> <version> [-min <version>]")
	}
	installer, version := os.Args[1], os.Args[2]
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	min := fs.String("min", "", "the oldest version that may keep working (older ones must update)")
	feedName := fs.String("feed", "nightly.json", "the feed to update (latest.json: DAWGit Pro's, before Nightly)")
	if err := fs.Parse(os.Args[3:]); err != nil {
		return err
	}
	signing, err := release.LoadKey(release.KeyPath())
	if err != nil {
		return err
	}
	sum, err := release.FileSHA256(installer)
	if err != nil {
		return err
	}
	m := release.Manifest{Version: version, SHA256: sum, MinVersion: *min}
	release.Sign(signing, &m)
	env := func(name string) (string, error) {
		v := strings.TrimSpace(os.Getenv(name))
		if v == "" {
			return "", fmt.Errorf("%s is not set (see README)", name)
		}
		return v, nil
	}
	var t s3put.Target
	var public string
	for _, f := range []struct {
		name string
		to   *string
	}{
		{"DAWGIT_RELEASE_ENDPOINT", &t.Endpoint}, {"DAWGIT_RELEASE_BUCKET", &t.Bucket},
		{"DAWGIT_RELEASE_ACCESS_KEY", &t.AccessKey}, {"DAWGIT_RELEASE_SECRET_KEY", &t.SecretKey},
		{"DAWGIT_RELEASE_PUBLIC_URL", &public},
	} {
		if *f.to, err = env(f.name); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(installer)
	if err != nil {
		return err
	}
	name := strings.ReplaceAll(filepath.Base(installer), " ", "-")
	key := "releases/" + version + "/" + name
	fmt.Printf("uploading %s (%.1f MB)…\n", key, float64(len(data))/(1<<20))
	if err := t.Put(key, data, "application/octet-stream"); err != nil {
		return err
	}
	feed, _ := json.MarshalIndent(map[string]string{
		"version":    version,
		"download":   strings.TrimRight(public, "/") + "/" + key,
		"sha256":     m.SHA256,
		"minVersion": m.MinVersion,
		"signature":  m.Signature,
	}, "", "  ")
	if err := t.Put(*feedName, feed, "application/json"); err != nil {
		return err
	}
	fmt.Printf("published %s\nfeed: %s/%s\n", version, strings.TrimRight(public, "/"), *feedName)
	return nil
}
