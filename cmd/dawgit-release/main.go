// Command dawgit-release signs DAWGit installers for automatic updates.
//
//	dawgit-release keygen                       make the signing key (once; prints the public key to build in)
//	dawgit-release sign <installer> <version> [-min <version>] [-o update.json]
//
// sign writes the release's manifest (version, the installer's SHA-256, the
// oldest version that may keep working, signature), uploaded next to the
// installer: update.json on a GitHub release. The key is at
// DAWGIT_SIGNING_KEY or in the user's config folder, never in a repository.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"dawgit/release"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "dawgit-release:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dawgit-release keygen | sign <installer> <version> [-min <version>] [-o <file>]")
	}
	switch args[0] {
	case "keygen":
		pub, err := release.NewKey(release.KeyPath())
		if err != nil {
			return err
		}
		fmt.Printf("signing key: %s (keep it safe, never commit it)\npublic key:  %s\n", release.KeyPath(), pub)
		return nil
	case "sign":
		fs := flag.NewFlagSet("sign", flag.ContinueOnError)
		min := fs.String("min", "", "the oldest version that may keep working (older ones must update)")
		out := fs.String("o", "", "where to write the manifest (default: update.json next to the installer)")
		if len(args) < 3 {
			return fmt.Errorf("usage: dawgit-release sign <installer> <version> [-min <version>] [-o <file>]")
		}
		if err := fs.Parse(args[3:]); err != nil {
			return err
		}
		installer, version := args[1], args[2]
		key, err := release.LoadKey(release.KeyPath())
		if err != nil {
			return err
		}
		sum, err := release.FileSHA256(installer)
		if err != nil {
			return err
		}
		m := release.Manifest{Version: version, SHA256: sum, MinVersion: *min}
		release.Sign(key, &m)
		path := *out
		if path == "" {
			path = filepath.Join(filepath.Dir(installer), "update.json")
		}
		data, _ := json.MarshalIndent(m, "", "  ")
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("signed %s %s -> %s\n", version, filepath.Base(installer), path)
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}
