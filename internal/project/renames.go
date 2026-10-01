package project

import (
	"bytes"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"dawgit/internal/textdiff"
)

// Versions don't record moves: like git, DAWGit finds them when comparing,
// pairing a file gone from one side with a file new on the other. Same
// content is a move; for text, mostly the same content (and the same kind
// of file) is a move with changes.

// renamed pairs a path in the old version with its path in the new one.
type renamed struct {
	from, to string
	same     bool // same content (else: similar text)
}

const (
	similarEnough  = 0.5     // share of lines kept, for text moved and changed
	maxSimilarSize = 1 << 20 // bytes: bigger text is only paired when unchanged
	maxSimilarWork = 2500    // pairs tried by content, at most
)

// findRenames pairs gone files with new ones. content reads a file's bytes
// (false when it isn't at hand: such files are paired only by hash).
func findRenames(gone, added []FileEntry, content func(f FileEntry, old bool) ([]byte, bool)) []renamed {
	var out []renamed
	// Same content: by hash, preferring the same file name.
	byHash := map[string][]FileEntry{}
	for _, g := range gone {
		byHash[g.Hash] = append(byHash[g.Hash], g)
	}
	usedGone := map[string]bool{}
	var restAdded []FileEntry
	for _, a := range added {
		cands := byHash[a.Hash]
		pick := -1
		for i, c := range cands {
			if !usedGone[c.Path] && (pick < 0 || path.Base(c.Path) == path.Base(a.Path)) {
				pick = i
				if path.Base(c.Path) == path.Base(a.Path) {
					break
				}
			}
		}
		if pick < 0 {
			restAdded = append(restAdded, a)
			continue
		}
		usedGone[cands[pick].Path] = true
		out = append(out, renamed{from: cands[pick].Path, to: a.Path, same: true})
	}
	// Similar text, same extension.
	var restGone []FileEntry
	for _, g := range gone {
		if !usedGone[g.Path] {
			restGone = append(restGone, g)
		}
	}
	if len(restGone)*len(restAdded) > maxSimilarWork || len(restGone) == 0 || len(restAdded) == 0 {
		return out
	}
	text := func(f FileEntry, old bool) ([]string, bool) {
		if f.Size > maxSimilarSize {
			return nil, false
		}
		data, ok := content(f, old)
		if !ok || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			return nil, false
		}
		return textdiff.Split(string(data)), true
	}
	type cand struct {
		g, a  int
		score float64
	}
	var cands []cand
	goneText := map[int][]string{}
	for i, g := range restGone {
		if t, ok := text(g, true); ok {
			goneText[i] = t
		}
	}
	for j, a := range restAdded {
		at, ok := text(a, false)
		if !ok {
			continue
		}
		for i, g := range restGone {
			gt, ok := goneText[i]
			if !ok || !strings.EqualFold(path.Ext(g.Path), path.Ext(a.Path)) {
				continue
			}
			if s := similarity(gt, at); s >= similarEnough {
				cands = append(cands, cand{i, j, s})
			}
		}
	}
	sort.Slice(cands, func(x, y int) bool { return cands[x].score > cands[y].score })
	takenG, takenA := map[int]bool{}, map[int]bool{}
	for _, c := range cands {
		if takenG[c.g] || takenA[c.a] {
			continue
		}
		takenG[c.g], takenA[c.a] = true, true
		out = append(out, renamed{from: restGone[c.g].Path, to: restAdded[c.a].Path})
	}
	return out
}

// similarity: the share of lines both keep (0 to 1).
func similarity(a, b []string) float64 {
	if len(a)+len(b) == 0 {
		return 1
	}
	same := 0
	for _, op := range textdiff.Diff(a, b) {
		if op.Kind == '=' {
			same++
		}
	}
	return 2 * float64(same) / float64(len(a)+len(b))
}

// storedContent reads a stored file's content when it is on this computer.
func (r *Repo) storedContent(f FileEntry) ([]byte, bool) {
	if !r.available(f.Hash) {
		return nil, false
	}
	o, err := r.openObject(f.Hash)
	if err != nil {
		return nil, false
	}
	defer o.Close()
	data, err := io.ReadAll(io.LimitReader(o, maxSimilarSize+1))
	return data, err == nil
}

// renamesBetween finds the moves from version a to version b.
func (r *Repo) renamesBetween(a, b *Manifest) []renamed {
	af, bf := a.FileMap(), b.FileMap()
	var gone, added []FileEntry
	for _, f := range a.Files {
		if _, ok := bf[f.Path]; !ok {
			gone = append(gone, f)
		}
	}
	for _, f := range b.Files {
		if _, ok := af[f.Path]; !ok {
			added = append(added, f)
		}
	}
	if len(gone) == 0 || len(added) == 0 {
		return nil
	}
	return findRenames(gone, added, func(f FileEntry, _ bool) ([]byte, bool) { return r.storedContent(f) })
}

// workingContent reads a file in the project folder.
func (r *Repo) workingContent(f FileEntry) ([]byte, bool) {
	fh, err := os.Open(r.Abs(f.Path))
	if err != nil {
		return nil, false
	}
	defer fh.Close()
	data, err := io.ReadAll(io.LimitReader(fh, maxSimilarSize+1))
	return data, err == nil
}
