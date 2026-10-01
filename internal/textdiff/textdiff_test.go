package textdiff

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// check verifies ops walk both a and b completely, in order, keeping only
// equal lines, and returns how many lines changed.
func check(t *testing.T, a, b []string, ops []Op) int {
	t.Helper()
	i, j, edits := 0, 0, 0
	for _, op := range ops {
		switch op.Kind {
		case '=':
			if op.A != i || op.B != j || a[i] != b[j] {
				t.Fatalf("bad keep %+v at %d,%d", op, i, j)
			}
			i, j = i+1, j+1
		case '-':
			if op.A != i {
				t.Fatalf("bad remove %+v at %d", op, i)
			}
			i, edits = i+1, edits+1
		case '+':
			if op.B != j {
				t.Fatalf("bad add %+v at %d", op, j)
			}
			j, edits = j+1, edits+1
		}
	}
	if i != len(a) || j != len(b) {
		t.Fatalf("ended at %d,%d of %d,%d", i, j, len(a), len(b))
	}
	return edits
}

// lcs is the textbook longest common subsequence length.
func lcs(a, b []string) int {
	dp := make([][]int, len(a)+1)
	for i := range dp {
		dp[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	return dp[0][0]
}

func TestDiffShortest(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := []string{"a", "b", "c", "d"}
	gen := func() []string {
		out := make([]string, rng.Intn(25))
		for i := range out {
			out[i] = words[rng.Intn(len(words))]
		}
		return out
	}
	for n := 0; n < 3000; n++ {
		a, b := gen(), gen()
		if got, want := check(t, a, b, Diff(a, b)), len(a)+len(b)-2*lcs(a, b); got != want {
			t.Fatalf("%v -> %v: %d edits, shortest is %d", a, b, got, want)
		}
	}
}

func TestDiffTooManyEdits(t *testing.T) {
	old := maxEdits
	defer func() { maxEdits = old }()
	maxEdits = 3
	a := strings.Split("x 1 2 3 4 5 y", " ")
	b := strings.Split("x 6 7 8 9 y", " ")
	ops := Diff(a, b)
	check(t, a, b, ops)
	if ops[0].Kind != '=' || ops[len(ops)-1].Kind != '=' || ops[1].Kind != '-' || ops[6].Kind != '+' {
		t.Fatalf("expected removed then added between the kept ends: %+v", ops)
	}
}

func TestSplit(t *testing.T) {
	for in, want := range map[string][]string{
		"": nil, "a": {"a"}, "a\n": {"a"}, "a\r\nb\r\n": {"a", "b"}, "a\n\nb": {"a", "", "b"},
	} {
		if got := Split(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q) = %q", in, got)
		}
	}
}

func TestHunks(t *testing.T) {
	var a []string
	for i := 1; i <= 30; i++ {
		a = append(a, string(rune('A'+i%26)))
	}
	b := append([]string(nil), a...)
	b[4] = "changed"                                          // line 5
	b = append(b[:20], append([]string{"new"}, b[20:]...)...) // after line 20
	hs := Hunks(a, b, 3)
	if len(hs) != 2 {
		t.Fatalf("%d hunks: %+v", len(hs), hs)
	}
	first := hs[0].Lines
	if first[0].Old != 2 || first[len(first)-1].Old != 8 {
		t.Fatalf("first hunk spans %d..%d", first[0].Old, first[len(first)-1].Old)
	}
	var kinds []string
	for _, l := range first {
		kinds = append(kinds, l.Kind)
	}
	if strings.Join(kinds, " ") != "same same same del add same same same" {
		t.Fatalf("first hunk: %v", kinds)
	}
	if l := hs[1].Lines[3]; l.Kind != "add" || l.New != 21 || l.Text != "new" {
		t.Fatalf("second hunk: %+v", hs[1].Lines)
	}
	// Changes 6 lines apart share a hunk.
	c := append([]string(nil), a...)
	c[4], c[10] = "x", "y"
	if hs := Hunks(a, c, 3); len(hs) != 1 {
		t.Fatalf("close changes: %d hunks", len(hs))
	}
	if Hunks(a, a, 3) != nil {
		t.Fatal("no changes, no hunks")
	}
}

func BenchmarkDiffBigFile(b *testing.B) {
	var x []string
	for i := 0; i < 100_000; i++ {
		x = append(x, strings.Repeat("y", i%40))
	}
	y := append([]string(nil), x...)
	for i := 0; i < len(y); i += 997 {
		y[i] = "edited"
	}
	for b.Loop() {
		Diff(x, y)
	}
}
