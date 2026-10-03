package project

import (
	"fmt"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"dawgit/internal/chunk"
	"dawgit/internal/remote"
	"dawgit/internal/remote/s3test"
)

// filesUnder reads the files under dir of a project (path -> content).
func filesUnder(t *testing.T, root, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	base := filepath.Join(root, dir)
	filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

// shuffler changes the files under Files/ of a project at random: moves,
// renames, overwrites, deletes, copies, swaps, and contents coming back.
type shuffler struct {
	rnd  *rand.Rand
	seen []string // contents ever written (to bring back old ones)
	n    int
	who  string
}

func (s *shuffler) content() string {
	if len(s.seen) > 0 && s.rnd.Intn(4) == 0 {
		return s.seen[s.rnd.Intn(len(s.seen))] // an old content again
	}
	s.n++
	c := fmt.Sprintf("%s content %d %s", s.who, s.n, strings.Repeat("x", s.rnd.Intn(50)))
	if s.rnd.Intn(16) == 0 { // a big file, kept as pieces in the team's storage
		c = bigBase()[:chunk.MinFile+s.rnd.Intn(1<<20)] + c
	}
	s.seen = append(s.seen, c)
	return c
}

var bigBaseOnce = sync.OnceValue(func() string { return string(randomBytes(9, chunk.MinFile+1<<20)) })

// bigBase is the start of the big files: they share most of their pieces.
func bigBase() string { return bigBaseOnce() }

func (s *shuffler) newPath() string {
	dirs := []string{"Files", "Files/A", "Files/B", "Files/A/Deep", "Files/C"}
	return fmt.Sprintf("%s/%s%d.wav", dirs[s.rnd.Intn(len(dirs))], s.who, s.rnd.Intn(12))
}

// step makes one random change; it returns what it did.
func (s *shuffler) step(t *testing.T, root string) string {
	t.Helper()
	files := filesUnder(t, root, "Files")
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	abs := func(p string) string { return filepath.Join(root, filepath.FromSlash(p)) }
	put := func(p, c string) {
		os.MkdirAll(filepath.Dir(abs(p)), 0o755)
		if err := os.WriteFile(abs(p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(paths) == 0 {
		p := s.newPath()
		put(p, s.content())
		return "add " + p
	}
	pick := paths[s.rnd.Intn(len(paths))]
	switch s.rnd.Intn(7) {
	case 0: // move
		to := s.newPath()
		if _, taken := files[to]; taken {
			return "nothing"
		}
		os.MkdirAll(filepath.Dir(abs(to)), 0o755)
		os.Rename(abs(pick), abs(to))
		return "move " + pick + " -> " + to
	case 1: // rename, and a new file where it was
		to := path.Join(path.Dir(pick), "old "+path.Base(pick))
		if _, taken := files[to]; taken {
			return "nothing"
		}
		os.Rename(abs(pick), abs(to))
		put(pick, s.content())
		return "rename " + pick + " -> " + to + ", new " + pick
	case 2: // overwrite
		put(pick, s.content())
		return "overwrite " + pick
	case 3: // delete
		os.Remove(abs(pick))
		return "delete " + pick
	case 4: // copy
		to := s.newPath()
		if _, taken := files[to]; taken {
			return "nothing"
		}
		put(to, files[pick])
		return "copy " + pick + " -> " + to
	case 5: // swap two files' contents
		other := paths[s.rnd.Intn(len(paths))]
		put(pick, files[other])
		put(other, files[pick])
		return "swap " + pick + " <-> " + other
	default: // add
		p := s.newPath()
		if _, taken := files[p]; taken {
			return "nothing"
		}
		put(p, s.content())
		return "add " + p
	}
}

// Random moves, renames, overwrites, deletes, copies and swaps by two people,
// with .dawgit keeping no copy of what the team has (files here are often
// the only copy): after every exchange both have the same files, with the
// right contents.
func TestShuffledFiles(t *testing.T) {
	seeds := 3
	if testing.Short() {
		seeds = 2
	}
	if n, err := strconv.Atoi(os.Getenv("DAWGIT_SHUFFLE_SEEDS")); err == nil {
		seeds = n // a longer run: DAWGIT_SHUFFLE_SEEDS=200
	}
	for seed := 1; seed <= seeds; seed++ {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) { shuffle(t, int64(seed)) })
	}
}

func shuffle(t *testing.T, seed int64) {
	fake := s3test.New("team")
	defer fake.Close()
	code := remote.EncodeConnectionCode(remote.Config{URL: "s3+" + fake.URL + "/team/dawgit",
		AccessKey: "key", SecretKey: "secret"})
	a, _ := Init(newProject(t), "yi")
	if err := a.SetRemote(code, ""); err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewSource(seed))
	sa := &shuffler{rnd: rnd, who: "a"}
	sb := &shuffler{rnd: rnd, who: "b"}
	for i := 0; i < 6; i++ {
		sa.step(t, a.Root)
	}
	if _, _, err := a.Save("first", Strategy("fail")); err != nil {
		t.Fatal(err)
	}
	b, _, err := Clone(code, "", "Song", filepath.Join(t.TempDir(), "B", "Song Project"), "alex")
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("seed %d: "+format+"\nwhat happened:\n  %s", append([]any{seed}, append(args, strings.Join(log, "\n  "))...)...)
	}
	for round := 0; round < 25; round++ {
		a.PruneObjects()
		b.PruneObjects()
		for i := rnd.Intn(4) + 1; i > 0; i-- {
			log = append(log, fmt.Sprintf("%d A: %s", round, sa.step(t, a.Root)))
		}
		if _, _, err := a.Save(fmt.Sprint("A ", round), Strategy("fail")); err != nil && !strings.Contains(err.Error(), "nothing") {
			fail("A save: %v", err)
		}
		if rnd.Intn(2) == 0 { // B worked too: B's save merges
			for i := rnd.Intn(3) + 1; i > 0; i-- {
				log = append(log, fmt.Sprintf("%d B: %s", round, sb.step(t, b.Root)))
			}
			if _, _, err := b.Save(fmt.Sprint("B ", round), Strategy("ours")); err != nil {
				fail("B save: %v", err)
			}
			if _, err := a.Update(Strategy("fail")); err != nil {
				fail("A update: %v", err)
			}
		} else if _, err := b.Update(Strategy("fail")); err != nil {
			fail("B update: %v", err)
		}
		fa, fb := filesUnder(t, a.Root, "Files"), filesUnder(t, b.Root, "Files")
		for p, c := range fa {
			if fb[p] != c {
				fail("round %d: %s on B is %q, on A %q", round, p, fb[p], c)
			}
		}
		for p := range fb {
			if _, ok := fa[p]; !ok {
				fail("round %d: %s only on B", round, p)
			}
		}
		// And what they have is what the version says.
		for _, r := range []*Repo{a, b} {
			m, _ := r.Load(r.Head())
			for _, f := range m.Files {
				if strings.HasPrefix(f.Path, "Files/") {
					if got := filesUnder(t, r.Root, "Files")[f.Path]; got == "" {
						fail("round %d: %s of the version missing on %s", round, f.Path, r.Config.Author)
					}
				}
			}
			if changes, _ := r.Status(); len(changes) != 0 {
				fail("round %d: %s not clean: %+v", round, r.Config.Author, changes)
			}
		}
	}
}
