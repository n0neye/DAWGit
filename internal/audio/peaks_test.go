package audio

import (
	"bytes"
	"testing"
)

func TestPeaks(t *testing.T) {
	// One second of stereo 16-bit silence, then half a second at full scale.
	in := make([]int16, 0, 2*66150)
	for i := 0; i < 44100; i++ {
		in = append(in, 0, 0)
	}
	for i := 0; i < 22050; i++ {
		in = append(in, 32767, -32768)
	}
	wav, err := AIFFToWAV(aiff(in))
	if err != nil {
		t.Fatal(err)
	}
	w, err := WAVPeaks(bytes.NewReader(wav), 3)
	if err != nil {
		t.Fatal(err)
	}
	if w.Duration != 1.5 {
		t.Errorf("duration %v", w.Duration)
	}
	if w.Min[0] != 0 || w.Max[0] != 0 || w.Max[1] != 0 {
		t.Errorf("silent slices: %v %v", w.Min, w.Max)
	}
	if w.Max[2] < 0.99 || w.Min[2] > -0.99 {
		t.Errorf("loud slice: min %v max %v", w.Min[2], w.Max[2])
	}
	if a, err := AIFFPeaks(aiff(in), 3); err != nil || a.Max[2] < 0.99 {
		t.Errorf("AIFF peaks: %v %+v", err, a)
	}
	if _, err := WAVPeaks(bytes.NewReader([]byte("nope")), 3); err == nil {
		t.Error("garbage accepted")
	}
}
