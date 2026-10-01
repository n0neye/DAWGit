package textmerge

import "testing"

func TestMerge(t *testing.T) {
	for _, c := range []struct {
		name, base, ours, theirs, want string
		clean                          bool
	}{
		{"both add elsewhere", "a\nb\nc\n", "x\na\nb\nc\n", "a\nb\nc\ny\n", "x\na\nb\nc\ny\n", true},
		{"edit different lines", "a\nb\nc\nd\n", "A\nb\nc\nd\n", "a\nb\nc\nD\n", "A\nb\nc\nD\n", true},
		{"same edit both sides", "a\nb\n", "a\nB\n", "a\nB\n", "a\nB\n", true},
		{"one side deletes", "a\nb\nc\n", "a\nc\n", "a\nb\nc\nd\n", "a\nc\nd\n", true},
		{"only theirs", "a\n", "a\n", "a\nb\n", "a\nb\n", true},
		{"no final newline", "a\nb", "a\nB", "z\na\nb", "z\na\nB", true},
		{"from empty, same text", "", "a\n", "a\n", "a\n", true},
		{"same line differently", "a\nb\nc\n", "a\nX\nc\n", "a\nY\nc\n", "", false},
		{"both insert at same place", "a\nc\n", "a\nb1\nc\n", "a\nb2\nc\n", "", false},
	} {
		got, clean, err := Merge([]byte(c.base), []byte(c.ours), []byte(c.theirs))
		if err != nil || clean != c.clean || (clean && string(got) != c.want) {
			t.Errorf("%s: clean=%v %q, want clean=%v %q", c.name, clean, got, c.clean, c.want)
		}
	}
}
