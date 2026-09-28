package diff

// A port of the parts of Python's difflib.SequenceMatcher used for device
// lists (no junk handling; autojunk only applies to sequences >= 200 items).

type opcode struct {
	tag            string // "replace" | "delete" | "insert" | "equal"
	i1, i2, j1, j2 int
}

type match struct{ a, b, size int }

func longestMatch(a, b []string, b2j map[string][]int, alo, ahi, blo, bhi int) match {
	best := match{alo, blo, 0}
	j2len := map[int]int{}
	for i := alo; i < ahi; i++ {
		newj2len := map[int]int{}
		for _, j := range b2j[a[i]] {
			if j < blo {
				continue
			}
			if j >= bhi {
				break
			}
			k := j2len[j-1] + 1
			newj2len[j] = k
			if k > best.size {
				best = match{i - k + 1, j - k + 1, k}
			}
		}
		j2len = newj2len
	}
	return best
}

func matchingBlocks(a, b []string) []match {
	b2j := map[string][]int{}
	for j, x := range b {
		b2j[x] = append(b2j[x], j)
	}
	type span struct{ alo, ahi, blo, bhi int }
	queue := []span{{0, len(a), 0, len(b)}}
	var blocks []match
	for len(queue) > 0 {
		q := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		m := longestMatch(a, b, b2j, q.alo, q.ahi, q.blo, q.bhi)
		if m.size > 0 {
			blocks = append(blocks, m)
			if q.alo < m.a && q.blo < m.b {
				queue = append(queue, span{q.alo, m.a, q.blo, m.b})
			}
			if m.a+m.size < q.ahi && m.b+m.size < q.bhi {
				queue = append(queue, span{m.a + m.size, q.ahi, m.b + m.size, q.bhi})
			}
		}
	}
	// sort by (a, b)
	for i := 1; i < len(blocks); i++ {
		for j := i; j > 0 && (blocks[j].a < blocks[j-1].a ||
			(blocks[j].a == blocks[j-1].a && blocks[j].b < blocks[j-1].b)); j-- {
			blocks[j], blocks[j-1] = blocks[j-1], blocks[j]
		}
	}
	// collapse adjacent blocks
	var out []match
	for _, m := range blocks {
		if n := len(out); n > 0 && out[n-1].a+out[n-1].size == m.a && out[n-1].b+out[n-1].size == m.b {
			out[n-1].size += m.size
		} else {
			out = append(out, m)
		}
	}
	return append(out, match{len(a), len(b), 0})
}

func opcodes(a, b []string) []opcode {
	var out []opcode
	i, j := 0, 0
	for _, m := range matchingBlocks(a, b) {
		tag := ""
		switch {
		case i < m.a && j < m.b:
			tag = "replace"
		case i < m.a:
			tag = "delete"
		case j < m.b:
			tag = "insert"
		}
		if tag != "" {
			out = append(out, opcode{tag, i, m.a, j, m.b})
		}
		i, j = m.a+m.size, m.b+m.size
		if m.size > 0 {
			out = append(out, opcode{"equal", m.a, i, m.b, j})
		}
	}
	return out
}
