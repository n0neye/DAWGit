package project

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"dawgit/internal/manifest"
	"dawgit/internal/remote"
	"dawgit/internal/store"
)

// Versions (format 2) keep the project folder as trees, one per folder
// (docs/design/tree-manifests.md). Here they live in .dawgit/trees, apart
// from file contents: they are the history, never pruned. In the team's
// storage they are objects like file contents.

const treesDir = "trees"

func (r *Repo) treePath(h string) string {
	return filepath.Join(r.Dir, treesDir, h[:2], h[2:])
}

// trees keeps parsed trees by hash. Versions share nearly all of them, so
// reading the next version of a project reads only the trees that changed.
var trees = &treeCache{m: map[string][]manifest.TreeEntry{}}

type treeCache struct {
	mu      sync.Mutex
	m       map[string][]manifest.TreeEntry
	entries int
}

// maxCachedEntries bounds the cache (a few hundred MB at most); past it the
// cache starts over.
const maxCachedEntries = 4_000_000

func (c *treeCache) get(h string) ([]manifest.TreeEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.m[h]
	return t, ok
}

func (c *treeCache) put(h string, t []manifest.TreeEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[h]; ok {
		return
	}
	if c.entries+len(t) > maxCachedEntries {
		c.m, c.entries = map[string][]manifest.TreeEntry{}, 0
	}
	c.m[h] = t
	c.entries += len(t)
}

// tree reads a tree stored here.
func (r *Repo) tree(h string) ([]manifest.TreeEntry, error) {
	if t, ok := trees.get(h); ok {
		return t, nil
	}
	data, err := os.ReadFile(r.treePath(h))
	if err != nil {
		return nil, fmt.Errorf("folder list %s of a version is missing: %w", short(h), err)
	}
	t, err := manifest.ParseTree(h, data)
	if err != nil {
		return nil, err
	}
	trees.put(h, t)
	return t, nil
}

// hasTree: stored in this project (the cache is shared by all projects).
func (r *Repo) hasTree(h string) bool {
	_, err := os.Stat(r.treePath(h))
	return err == nil
}

// writeTrees stores the trees not stored yet.
func (r *Repo) writeTrees(ts map[string][]byte) error {
	for h, data := range ts {
		if r.hasTree(h) {
			continue
		}
		if err := store.WriteAtomic(r.treePath(h), bytes.NewReader(data)); err != nil {
			return err
		}
	}
	return nil
}

// fillFiles lists a format-2 version's files from its trees.
func (r *Repo) fillFiles(m *Manifest) error {
	if m.Tree == "" {
		return nil
	}
	files, err := manifest.Flatten(m.Tree, r.tree, nil)
	if err != nil {
		return fmt.Errorf("version %s: %w", short(m.ID), err)
	}
	m.Files = files
	return nil
}

// sealTrees writes a new format-2 version's trees from its files and
// records the top one.
func (r *Repo) sealTrees(m *Manifest) error {
	root, ts, err := manifest.BuildTrees(m.Files)
	if err != nil {
		return err
	}
	// The parents' trees are stored already (most of them, usually).
	for _, p := range m.Parents {
		if h, err := r.Header(p); err == nil && h.Tree != "" {
			r.walkTrees([]string{h.Tree}, func(t string, _ []manifest.TreeEntry) error {
				delete(ts, t)
				return nil
			})
		}
	}
	if err := r.writeTrees(ts); err != nil {
		return err
	}
	m.Tree, m.FileCount, m.TotalSize = root, len(m.Files), 0
	for _, f := range m.Files {
		m.TotalSize += f.Size
	}
	return nil
}

// walkTrees calls fn on every tree under the roots, each once.
func (r *Repo) walkTrees(roots []string, fn func(h string, entries []manifest.TreeEntry) error) error {
	seen := map[string]bool{}
	stack := append([]string(nil), roots...)
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[h] {
			continue
		}
		seen[h] = true
		entries, err := r.tree(h)
		if err != nil {
			return err
		}
		if err := fn(h, entries); err != nil {
			return err
		}
		for _, e := range entries {
			if e.Dir && !seen[e.Hash] {
				stack = append(stack, e.Hash)
			}
		}
	}
	return nil
}

// uploadTrees puts the trees under roots that the team's storage lacks.
func (r *Repo) uploadTrees(c remote.Backend, roots []string) error {
	if len(roots) == 0 {
		return nil
	}
	var all []string
	if err := r.walkTrees(roots, func(h string, _ []manifest.TreeEntry) error {
		all = append(all, h)
		return nil
	}); err != nil {
		return err
	}
	missing, err := c.MissingObjects(all)
	if err != nil {
		return err
	}
	return inParallel(missing, func(h string) error {
		data, err := os.ReadFile(r.treePath(h))
		if err != nil {
			return err
		}
		return c.PutObject(h, bytes.NewReader(data))
	})
}

// fetchTrees downloads the trees under root not stored here, a level of
// folders at a time.
func (r *Repo) fetchTrees(c remote.Backend, root string) error {
	seen := map[string]bool{}
	level := []string{root}
	for len(level) > 0 {
		var need []string
		for _, h := range level {
			if !r.hasTree(h) {
				need = append(need, h)
			}
		}
		if err := inParallel(need, func(h string) error {
			body, err := c.GetObject(h)
			if err != nil {
				return fmt.Errorf("download folder list %s: %w", short(h), err)
			}
			data, err := io.ReadAll(body)
			body.Close()
			if err != nil {
				return err
			}
			t, err := manifest.ParseTree(h, data)
			if err != nil {
				return err
			}
			if err := store.WriteAtomic(r.treePath(h), bytes.NewReader(data)); err != nil {
				return err
			}
			trees.put(h, t)
			return nil
		}); err != nil {
			return err
		}
		var next []string
		for _, h := range level {
			entries, err := r.tree(h)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Dir && !seen[e.Hash] {
					seen[e.Hash] = true
					next = append(next, e.Hash)
				}
			}
		}
		level = next
	}
	return nil
}
