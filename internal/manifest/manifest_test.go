package manifest

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func hashOf(s string) string { return ID([]byte(s)) }

func TestTreesRoundTrip(t *testing.T) {
	files := []FileEntry{
		{Path: "a.b", Hash: hashOf("1"), Size: 1},
		{Path: "a/c", Hash: hashOf("2"), Size: 2},
		{Path: "Assets/My Scene.unity", Hash: hashOf("3"), Size: 3},
		{Path: "Assets/Art/x.png", Hash: hashOf("4"), Size: 4},
		{Path: "top.txt", Hash: hashOf("5"), Size: 0},
	}
	root, trees, err := BuildTrees(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(trees) != 4 { // top, a, Assets, Assets/Art
		t.Fatalf("%d trees", len(trees))
	}
	got, err := Flatten(root, func(h string) ([]TreeEntry, error) { return ParseTree(h, trees[h]) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]FileEntry(nil), files...)
	sortFiles(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	// Same files, same trees: unchanged folders keep their hash.
	files[0].Hash = hashOf("changed")
	root2, trees2, _ := BuildTrees(files)
	if root2 == root {
		t.Fatal("a change must change the top tree")
	}
	shared := 0
	for h := range trees2 {
		if trees[h] != nil {
			shared++
		}
	}
	if shared != 3 { // a.b is at the top: a/, Assets/ and Assets/Art/ stay
		t.Fatalf("shared trees: %d", shared)
	}
	if _, _, err := BuildTrees([]FileEntry{{Path: "bad\nname", Hash: hashOf("x")}}); err == nil {
		t.Fatal("line breaks in names must fail")
	}
	empty, trees3, _ := BuildTrees(nil)
	if got, err := Flatten(empty, func(h string) ([]TreeEntry, error) { return ParseTree(h, trees3[h]) }, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty project: %v %v", got, err)
	}
}

func sortFiles(fs []FileEntry) {
	for i := range fs {
		for j := i + 1; j < len(fs); j++ {
			if fs[j].Path < fs[i].Path {
				fs[i], fs[j] = fs[j], fs[i]
			}
		}
	}
}

func TestParseTreeRejectsBadInput(t *testing.T) {
	good := EncodeTree([]TreeEntry{{Name: "x", Hash: hashOf("x"), Size: 1}})
	if _, err := ParseTree(hashOf("other"), good); err == nil {
		t.Fatal("wrong hash")
	}
	for _, bad := range []string{"f abc 1 x\n", "f " + hashOf("x") + " -1 x\n", "d " + hashOf("x") + " a/b\n",
		"q " + hashOf("x") + " x\n", "f " + hashOf("x") + " 1 x"} {
		if _, err := ParseTree(hashOf(bad), []byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestFormat2Record(t *testing.T) {
	m := &Manifest{Version: 2, Parents: []string{}, Author: "yi", Time: "2026-01-01T00:00:00Z", Message: "m",
		Tree: hashOf("tree"), FileCount: 3, TotalSize: 42}
	data := m.Seal()
	got, err := Parse(m.ID, data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tree != m.Tree || got.FileCount != 3 || got.TotalSize != 42 || got.Files != nil {
		t.Fatalf("%+v", got)
	}
	// A DAWGit that knows only format 1 fails on it instead of reading an
	// empty project.
	var old struct {
		Files []FileEntry `json:"files"`
	}
	if err := json.Unmarshal(data, &old); err == nil {
		t.Fatal("format 1 readers must not read a format-2 record")
	}
	// Format 1 still reads.
	v1 := &Manifest{Version: 1, Parents: []string{}, Files: []FileEntry{{Path: "a", Hash: hashOf("a"), Size: 1}}}
	d1 := v1.Seal()
	if got, err := Parse(v1.ID, d1); err != nil || len(got.Files) != 1 || got.Tree != "" {
		t.Fatalf("format 1: %+v %v", got, err)
	}
	// Newer formats are refused.
	d3 := []byte(strings.Replace(string(data), `"version": 2`, `"version": 3`, 1))
	if _, err := Parse(ID(d3), d3); !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("format 3: %v", err)
	}
}
