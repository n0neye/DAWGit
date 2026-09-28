package livecheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessNamesIncludesSelf(t *testing.T) {
	names, err := processNames()
	if err != nil {
		t.Fatal(err)
	}
	self := strings.ToLower(filepath.Base(os.Args[0]))
	for _, n := range names {
		if strings.ToLower(filepath.Base(strings.TrimSpace(n))) == self {
			return
		}
	}
	t.Fatalf("own process %q not among %d processes", self, len(names))
}
