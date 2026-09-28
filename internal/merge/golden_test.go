package merge

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"dawgit/internal/als"
	"dawgit/internal/diff"
)

// Golden data comes from the Python reference implementation:
//
//	cd python && python -m tests.make_golden
var goldenDir = filepath.Join("..", "..", "testdata", "golden")
var repoRoot = filepath.Join("..", "..")

func requireGolden(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(goldenDir); err != nil {
		t.Skip("no golden data; run: cd python && python -m tests.make_golden")
	}
}

func load(t *testing.T, path string) *als.LiveSet {
	t.Helper()
	s, err := als.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readGz(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func firstDiff(a, b []byte) string {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	lo := max(0, i-200)
	return "at byte " + strconv.Itoa(i) + "\nwant: " + string(a[lo:min(len(a), i+100)]) + "\n got: " + string(b[lo:min(len(b), i+100)])
}

func TestGoldenMerge(t *testing.T) {
	requireGolden(t)
	cases, err := os.ReadDir(filepath.Join(goldenDir, "merge"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		dir := filepath.Join(goldenDir, "merge", c.Name())
		t.Run(c.Name(), func(t *testing.T) {
			strategy, _ := os.ReadFile(filepath.Join(dir, "strategy.txt"))
			r, err := Merge(load(t, filepath.Join(dir, "base.als")), load(t, filepath.Join(dir, "ours.als")),
				load(t, filepath.Join(dir, "theirs.als")), string(strategy))
			if err != nil {
				t.Fatal(err)
			}
			wantReport, _ := os.ReadFile(filepath.Join(dir, "report.txt"))
			// Python's write_text uses CRLF on Windows.
			if got := r.Report(); got != strings.ReplaceAll(string(wantReport), "\r\n", "\n") {
				t.Errorf("report differs\n--- want\n%s\n--- got\n%s", wantReport, got)
			}
			want := readGz(t, filepath.Join(dir, "expected.xml.gz"))
			if got := r.Merged.XML(); !bytes.Equal(got, want) {
				t.Errorf("merged XML differs %s", firstDiff(want, got))
			}
		})
	}
}

func TestGoldenDiff(t *testing.T) {
	requireGolden(t)
	var cases []struct{ A, B, Expected string }
	data, err := os.ReadFile(filepath.Join(goldenDir, "diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	cache := map[string]*als.LiveSet{}
	get := func(p string) *als.LiveSet {
		if s, ok := cache[p]; ok {
			return s
		}
		s := load(t, filepath.Join(repoRoot, filepath.FromSlash(p)))
		cache[p] = s
		return s
	}
	failed := 0
	for _, c := range cases {
		if got := diff.Diff(get(c.A), get(c.B)).Render(); got != c.Expected {
			failed++
			if failed <= 3 {
				t.Errorf("%s -> %s\n--- want\n%s\n--- got\n%s", c.A, c.B, c.Expected, got)
			}
		}
	}
	if failed > 0 {
		t.Errorf("%d/%d diff cases differ", failed, len(cases))
	}
}

func TestGoldenValidate(t *testing.T) {
	requireGolden(t)
	var cases []struct {
		File   string
		Issues []string
	}
	data, err := os.ReadFile(filepath.Join(goldenDir, "validate.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got := als.Validate(load(t, filepath.Join(repoRoot, filepath.FromSlash(c.File))))
		if strings.Join(got, "\n") != strings.Join(c.Issues, "\n") {
			t.Errorf("%s: want %v, got %v", c.File, c.Issues, got)
		}
	}
}
