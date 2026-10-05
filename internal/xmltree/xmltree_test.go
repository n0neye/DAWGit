package xmltree

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func fixtures(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "live")
	var files []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Ext(p) == ".als" &&
			!bytes.HasPrefix([]byte(d.Name()), []byte("MergeTest-")) {
			files = append(files, p)
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	return files
}

func gunzip(t *testing.T, path string) []byte {
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
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRoundTripIsByteExact(t *testing.T) {
	for _, path := range fixtures(t) {
		raw := gunzip(t, path)
		root, err := Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		out := Serialize(root)
		if !bytes.Equal(out, raw) {
			i := 0
			for i < len(out) && i < len(raw) && out[i] == raw[i] {
				i++
			}
			lo := max(0, i-80)
			t.Fatalf("%s: differs at byte %d\nwant: %q\ngot:  %q", filepath.Base(path), i,
				raw[lo:min(len(raw), i+40)], out[lo:min(len(out), i+40)])
		}
	}
}

func TestNavigation(t *testing.T) {
	root, err := Parse([]byte(`<A><B Id="1"><C Value="x" /></B><B Id="2" /></A>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := root.Val("B/C", ""); got != "x" {
		t.Errorf("Val = %q", got)
	}
	if n := len(root.FindAll("B")); n != 2 {
		t.Errorf("FindAll = %d", n)
	}
	if n := len(root.Iter("")); n != 4 {
		t.Errorf("Iter = %d", n)
	}
	c := root.Clone()
	c.Children[0].Set("Id", "9")
	if root.Children[0].Attr("Id") != "1" {
		t.Error("Clone is shallow")
	}
}
