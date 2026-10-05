package project

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// A share cut off at any step — the connection lost, DAWGit closed — never
// leaves the team's data broken: a teammate still gets a whole version (the
// one before, or the new one), and sharing again finishes the job. Every
// step is tried, one cut at a time.
func TestShareCutAtEveryStep(t *testing.T) {
	if testing.Short() {
		t.Skip("cuts a share at every step")
	}
	// Each try on storage of its own: contents shared earlier would be
	// there already (stored by hash) and the share would write less.
	team := func() (*s3test.Server, string) {
		fake := s3test.New("team")
		t.Cleanup(fake.Close)
		return fake, remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
			AccessKey: "key", SecretKey: "secret"})
	}

	// How many writes a share makes, measured once without a cut.
	fake, code := team()
	a, _, after := sharedProject(t, code, "Count")
	start := fake.Writes
	if _, _, err := a.Save("change", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	if got := cloneFiles(t, code, a.Config.Name); !maps.Equal(got, after) {
		t.Fatal("an uncut share doesn't give the new version")
	}
	steps := fake.Writes - start
	if steps < 5 {
		t.Fatalf("a share made only %d writes", steps)
	}
	for cut := 1; cut < steps; cut++ {
		fake, code := team()
		a, before, after := sharedProject(t, code, "Song")
		fake.CutAfter = fake.Writes + cut
		_, _, err := a.Save("change", Strategy("fail"))
		fake.CutAfter = 0
		shared := err == nil
		// A teammate gets a whole version: the one before or the new one.
		got := cloneFiles(t, code, a.Config.Name)
		if !maps.Equal(got, before) && !maps.Equal(got, after) {
			t.Fatalf("cut after %d of %d writes: a clone gets neither version:\n%v", cut, steps, fileNames(got))
		}
		if shared && !maps.Equal(got, after) {
			t.Fatalf("cut after %d of %d writes: said shared, but a clone gets the version before", cut, steps)
		}
		// Sharing again finishes it.
		if _, _, err := a.Save("again", Strategy("fail")); err != nil {
			t.Fatalf("cut after %d of %d writes: sharing again: %v", cut, steps, err)
		}
		if got := cloneFiles(t, code, a.Config.Name); !maps.Equal(got, after) {
			t.Fatalf("cut after %d of %d writes: after sharing again, a clone gets:\n%v", cut, steps, fileNames(got))
		}
	}
}

func fileNames(m map[string]string) []string {
	out := slices.Collect(maps.Keys(m))
	slices.Sort(out)
	return out
}

// sharedProject makes a project named name, shares a first version, then
// changes it (not yet saved); it returns the files before and after.
func sharedProject(t *testing.T, code, name string) (*Repo, map[string]string, map[string]string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), name+" Project")
	os.MkdirAll(filepath.Join(root, "Ableton Project Info"), 0o755)
	a, err := Init(root, "yi")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	write(t, root, "notes.txt", "first")
	write(t, root, "Samples/kick.wav", string(randomBytes(1, 300<<10)))
	write(t, root, "Samples/pad.wav", string(randomBytes(2, 300<<10)))
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	before := files(t, root)
	write(t, root, "notes.txt", "second")
	write(t, root, "Samples/snare.wav", string(randomBytes(3, 300<<10)))
	write(t, root, "Maps/Level.umap", string(randomBytes(4, 17<<20))) // kept as pieces
	os.Remove(filepath.Join(root, "Samples", "pad.wav"))
	return a, before, files(t, root)
}

// files maps a project folder's files (not .dawgit) to their contents.
func files(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".dawgit" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel == ".dawgit.yaml" || strings.HasPrefix(rel, "Ableton Project Info/") {
			return nil
		}
		data, _ := os.ReadFile(p)
		out[rel] = string(data)
		return nil
	})
	return out
}

// cloneFiles clones the team's project into a new folder and returns its
// files.
func cloneFiles(t *testing.T, code, project string) map[string]string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "B", project+" Project")
	c, _, err := Clone(code, "", project, dir, "alex")
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	return files(t, c.Root)
}
