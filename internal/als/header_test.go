package als

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreatorOf(t *testing.T) {
	set := filepath.Join("..", "..", "SampleProjects", "SampleAbletonProject Project", "SampleAbletonProject_v3.als")
	if _, err := os.Stat(set); err != nil {
		t.Skip("fixture not present")
	}
	if got := CreatorOf(set); got != "Ableton Live 12.3.1" {
		t.Errorf("CreatorOf = %q", got)
	}
	if got := CreatorOf("no-such.als"); got != "" {
		t.Errorf("missing file: %q", got)
	}
}
