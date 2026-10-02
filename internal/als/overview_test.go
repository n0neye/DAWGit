package als

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverview(t *testing.T) {
	path := filepath.Join("..", "..", "SampleProjects", "SampleAbletonProject Project", "MergeTest-1-both-add-tracks.als")
	if _, err := os.Stat(path); err != nil {
		t.Skip("fixture not present")
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	o := s.Overview()
	if o.Tempo != 111 || o.TimeSig != [2]int{4, 4} || len(o.Scenes) != 8 || o.Length != 64 {
		t.Fatalf("set: tempo %v, %v, %d scenes, length %v", o.Tempo, o.TimeSig, len(o.Scenes), o.Length)
	}
	if len(o.Tracks) != 8 {
		t.Fatalf("%d tracks", len(o.Tracks))
	}
	bass := o.Tracks[0]
	if bass.Kind != "midi" || bass.Name != "1-Basic Saturated Bass" || bass.Color != 8 || !bass.Muted ||
		len(bass.Clips) != 1 || bass.Clips[0].Slot != -1 || bass.Clips[0].Color != 14 {
		t.Errorf("bass: %+v", bass)
	}
	if beat := o.Tracks[5]; beat.Kind != "audio" || beat.Volume != -5 || beat.Muted {
		t.Errorf("bounce: %+v", beat)
	}
	if o.Tracks[6].Kind != "return" || o.Main.Kind != "main" {
		t.Errorf("return %+v, main %+v", o.Tracks[6], o.Main)
	}
}

func TestTimeSignature(t *testing.T) {
	for v, want := range map[int][2]int{201: {4, 4}, 200: {3, 4}, 302: {6, 8}, 100: {2, 2}} {
		if got := timeSignature(v); got != want {
			t.Errorf("%d: %v, want %v", v, got, want)
		}
	}
}
