// Package textmerge merges text files line by line (a three-way merge, like
// diff3): changes made on one side are taken; where both sides changed the
// same lines differently, the merge isn't clean.
package textmerge

import "strings"

// maxCells bounds the line-matching work; bigger files are not merged.
const maxCells = 40_000_000

// Merge merges ours and theirs, both changed from base. clean is false when
// the changes touch the same lines differently, or the files are too big.
func Merge(base, ours, theirs []byte) (merged []byte, clean bool, err error) {
	b, o, t := lines(base), lines(ours), lines(theirs)
	if len(b)*len(o) > maxCells || len(b)*len(t) > maxCells {
		return nil, false, nil
	}
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

// match maps each line of a to its line in b along a longest common
// subsequence (-1 when unmatched).
func match(a, b []string) []int {
	n, m := len(a), len(b)
	// lcs[i][j]: LCS length of a[i:] and b[j:], one row at a time from the end.
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	out := make([]int, n)
	for i := range out {
		out[i] = -1
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out[i] = j
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	return out
}
