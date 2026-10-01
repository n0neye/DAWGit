package manifest

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TreeEntry is a file or a folder in a tree.
type TreeEntry struct {
	Name string
	Dir  bool   // a folder: Hash is its tree
	Hash string // a file's content, or a folder's tree
	Size int64  // a file's size
}

// EncodeTree writes a folder's entries, one line each, sorted by name:
//
//	d <tree hash> <name>
//	f <content hash> <size> <name>
func EncodeTree(entries []TreeEntry) []byte {
	sorted := append([]TreeEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	var b bytes.Buffer
	for _, e := range sorted {
		if e.Dir {
			fmt.Fprintf(&b, "d %s %s\n", e.Hash, e.Name)
		} else {
			fmt.Fprintf(&b, "f %s %d %s\n", e.Hash, e.Size, e.Name)
		}
	}
	return b.Bytes()
}

// ParseTree reads a tree and checks it against its hash.
func ParseTree(hash string, data []byte) ([]TreeEntry, error) {
	if got := ID(data); got != hash {
		return nil, fmt.Errorf("tree %s: content hash is %s", short(hash), short(got))
	}
	text := string(data)
	if text == "" {
		return nil, nil
	}
	if !strings.HasSuffix(text, "\n") {
		return nil, fmt.Errorf("tree %s: unterminated line", short(hash))
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	out := make([]TreeEntry, 0, len(lines))
	for _, l := range lines {
		var e TreeEntry
		var ok bool
		switch {
		case strings.HasPrefix(l, "d "):
			parts := strings.SplitN(l, " ", 3)
			if ok = len(parts) == 3; ok {
				e = TreeEntry{Name: parts[2], Dir: true, Hash: parts[1]}
			}
		case strings.HasPrefix(l, "f "):
			parts := strings.SplitN(l, " ", 4)
			if ok = len(parts) == 4; ok {
				size, err := strconv.ParseInt(parts[2], 10, 64)
				ok = err == nil && size >= 0
				e = TreeEntry{Name: parts[3], Hash: parts[1], Size: size}
			}
		}
		if !ok || e.Name == "" || strings.Contains(e.Name, "/") || !validHash(e.Hash) {
			return nil, fmt.Errorf("tree %s: bad line %q", short(hash), l)
		}
		out = append(out, e)
	}
	return out, nil
}

// BuildTrees turns files (slash-separated paths) into trees, one per
// folder. It returns the top tree's hash and every tree by hash.
func BuildTrees(files []FileEntry) (string, map[string][]byte, error) {
	type folder struct {
		files []TreeEntry
		dirs  map[string]*folder
	}
	newFolder := func() *folder { return &folder{dirs: map[string]*folder{}} }
	top := newFolder()
	for _, f := range files {
		if strings.ContainsAny(f.Path, "\n\r") {
			return "", nil, fmt.Errorf("%q: names with line breaks can't be kept in a version", f.Path)
		}
		parts := strings.Split(f.Path, "/")
		node := top
		for _, p := range parts[:len(parts)-1] {
			next := node.dirs[p]
			if next == nil {
				next = newFolder()
				node.dirs[p] = next
			}
			node = next
		}
		node.files = append(node.files, TreeEntry{Name: parts[len(parts)-1], Hash: f.Hash, Size: f.Size})
	}
	trees := map[string][]byte{}
	var write func(n *folder) string
	write = func(n *folder) string {
		entries := n.files
		for name, sub := range n.dirs {
			entries = append(entries, TreeEntry{Name: name, Dir: true, Hash: write(sub)})
		}
		data := EncodeTree(entries)
		h := ID(data)
		trees[h] = data
		return h
	}
	return write(top), trees, nil
}

// Flatten lists the files under the tree root (sorted by path), reading
// trees with get. visit, when not nil, sees every tree hash once.
func Flatten(root string, get func(hash string) ([]TreeEntry, error), visit func(hash string)) ([]FileEntry, error) {
	var out []FileEntry
	var walk func(hash, prefix string) error
	walk = func(hash, prefix string) error {
		if visit != nil {
			visit(hash)
		}
		entries, err := get(hash)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.Dir {
				if err := walk(e.Hash, prefix+e.Name+"/"); err != nil {
					return err
				}
			} else {
				out = append(out, FileEntry{Path: prefix + e.Name, Hash: e.Hash, Size: e.Size})
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
