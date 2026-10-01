// Package textmerge merges text files line by line (a three-way merge, like
// diff3): changes made on one side are taken; where both sides changed the
// same lines differently, the merge isn't clean.
package textmerge

import (
	"strings"

	"dawgit/internal/textdiff"
)

// Merge merges ours and theirs, both changed from base. clean is false when
// the changes touch the same lines differently.
func Merge(base, ours, theirs []byte) (merged []byte, clean bool, err error) {
	b, o, t := lines(base), lines(ours), lines(theirs)
	mo, mt := match(b, o), match(b, t)
	var out []string
	i, j, k := 0, 0, 0
	for i < len(b) || j < len(o) || k < len(t) {
		// The same base line on both sides, where both are now: unchanged.
		if i < len(b) && mo[i] == j && mt[i] == k {
			out = append(out, b[i])
			i, j, k = i+1, j+1, k+1
			continue
		}
		// The next base line both sides still have ends this chunk.
		i2, j2, k2 := len(b), len(o), len(t)
		for x := i; x < len(b); x++ {
			if mo[x] >= j && mt[x] >= k {
				i2, j2, k2 = x, mo[x], mt[x]
				break
			}
		}
		bc, oc, tc := b[i:i2], o[j:j2], t[k:k2]
		switch {
		case equal(oc, bc):
			out = append(out, tc...)
		case equal(tc, bc), equal(oc, tc):
			out = append(out, oc...)
		default:
			return nil, false, nil
		}
		i, j, k = i2, j2, k2
	}
	return []byte(strings.Join(out, "")), true, nil
}

func lines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.SplitAfter(string(data), "\n")
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// match maps each line of a to its line in b along a shortest diff (-1 when
// unmatched).
func match(a, b []string) []int {
	out := make([]int, len(a))
	for i := range out {
		out[i] = -1
	}
	for _, op := range textdiff.Diff(a, b) {
		if op.Kind == '=' {
			out[op.A] = op.B
		}
	}
	return out
}
