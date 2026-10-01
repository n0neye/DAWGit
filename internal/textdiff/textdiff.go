// Package textdiff compares text line by line (Myers' diff): which lines
// stayed, which were removed and which were added, and those changes in
// hunks with a few lines around them, as a diff viewer shows them.
package textdiff

import "strings"

// maxEdits bounds the work for one comparison. Beyond it the changed middle
// of the files is reported as all removed, then all added: still a correct
// diff, only a coarse one.
var maxEdits = 4000

// Op is one step from a to b.
type Op struct {
	Kind byte // '=' kept, '-' removed from a, '+' added from b
	A, B int  // line in a ('=' and '-'), line in b ('=' and '+'); -1 otherwise
}

// Diff returns the steps that turn a into b, in order.
func Diff(a, b []string) []Op {
	// Lines as numbers: comparing them is then cheap.
	ids := map[string]int32{}
	num := func(xs []string) []int32 {
		out := make([]int32, len(xs))
		for i, x := range xs {
			id, ok := ids[x]
			if !ok {
				id = int32(len(ids))
				ids[x] = id
			}
			out[i] = id
		}
		return out
	}
	na, nb := num(a), num(b)

	// The same lines at the start and at the end need no search.
	pre := 0
	for pre < len(na) && pre < len(nb) && na[pre] == nb[pre] {
		pre++
	}
	suf := 0
	for suf < len(na)-pre && suf < len(nb)-pre && na[len(na)-1-suf] == nb[len(nb)-1-suf] {
		suf++
	}
	ops := make([]Op, 0, len(a)+len(b))
	for i := 0; i < pre; i++ {
		ops = append(ops, Op{'=', i, i})
	}
	ma, mb := na[pre:len(na)-suf], nb[pre:len(nb)-suf]
	if kinds, ok := myers(ma, mb); ok {
		i, j := pre, pre
		for _, k := range kinds {
			switch k {
			case '=':
				ops = append(ops, Op{'=', i, j})
				i, j = i+1, j+1
			case '-':
				ops = append(ops, Op{'-', i, -1})
				i++
			default:
				ops = append(ops, Op{'+', -1, j})
				j++
			}
		}
	} else {
		for i := range ma {
			ops = append(ops, Op{'-', pre + i, -1})
		}
		for j := range mb {
			ops = append(ops, Op{'+', -1, pre + j})
		}
	}
	for i := 0; i < suf; i++ {
		ops = append(ops, Op{'=', len(a) - suf + i, len(b) - suf + i})
	}
	return ops
}

// myers finds a shortest edit script from a to b ('=', '-', '+' per step),
// or reports false when it takes more than maxEdits removals and additions.
func myers(a, b []int32) ([]byte, bool) {
	n, m := len(a), len(b)
	limit := min(n+m, maxEdits)
	off := limit + 1
	v := make([]int32, 2*limit+3) // furthest x on each diagonal k = x - y
	// trace[d]: v[-d-1 .. d+1] as it was before step d.
	var trace [][]int32
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int32(nil), v[off-d-1:off+d+2]...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = int(v[off+k+1]) // down: a line added
			} else {
				x = int(v[off+k-1]) + 1 // right: a line removed
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[off+k] = int32(x)
			if x >= n && y >= m {
				return backtrack(trace, n, m), true
			}
		}
	}
	return nil, false
}

func backtrack(trace [][]int32, n, m int) []byte {
	var out []byte
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		v := trace[d]
		at := func(k int) int { return int(v[k+d+1]) }
		k := x - y
		var pk int
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := at(pk)
		py := px - pk
		for x > px && y > py {
			out = append(out, '=')
			x, y = x-1, y-1
		}
		if d > 0 {
			if x == px {
				out = append(out, '+')
				y--
			} else {
				out = append(out, '-')
				x--
			}
		}
		x, y = px, py
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Split cuts text into lines without their line endings.
func Split(text string) []string {
	if text == "" {
		return nil
	}
	ls := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range ls {
		ls[i] = strings.TrimSuffix(l, "\r")
	}
	return ls
}

// Line is one line of a hunk. Old and New count from 1 (0: not on that side).
type Line struct {
	Kind string `json:"kind"` // same | add | del
	Old  int    `json:"old"`
	New  int    `json:"new"`
	Text string `json:"text"`
}

// Hunk is a run of changes with up to context unchanged lines around it.
type Hunk struct {
	Lines []Line `json:"lines"`
}

// Hunks groups the changes from a to b; changes closer than 2*context lines
// share a hunk.
func Hunks(a, b []string, context int) []Hunk {
	ops := Diff(a, b)
	var out []Hunk
	for i := 0; i < len(ops); {
		if ops[i].Kind == '=' {
			i++
			continue
		}
		start := max(i-context, 0)
		// Extend while the next change is close enough.
		end := i
		for end < len(ops) {
			if ops[end].Kind != '=' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].Kind == '=' {
				run++
			}
			if run == len(ops) || run-end > 2*context {
				end = min(end+context, len(ops))
				break
			}
			end = run
		}
		var h Hunk
		for _, op := range ops[start:end] {
			switch op.Kind {
			case '=':
				h.Lines = append(h.Lines, Line{"same", op.A + 1, op.B + 1, b[op.B]})
			case '-':
				h.Lines = append(h.Lines, Line{"del", op.A + 1, 0, a[op.A]})
			default:
				h.Lines = append(h.Lines, Line{"add", 0, op.B + 1, b[op.B]})
			}
		}
		out = append(out, h)
		i = end
	}
	return out
}
