package convert

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"dawgit/internal/audio"
)

// tone writes a 1-second stereo 16-bit 44.1 kHz sine WAV.
func tone(t *testing.T, dir string) string { return toneAt(t, dir, 44100) }

func toneAt(t *testing.T, dir string, rate int) string {
	t.Helper()
	frames := rate
	data := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		v := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		binary.LittleEndian.PutUint16(data[4*i:], uint16(v))
		binary.LittleEndian.PutUint16(data[4*i+2:], uint16(v))
	}
	h := make([]byte, 44)
	le := binary.LittleEndian
	copy(h, "RIFF")
	le.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	le.PutUint32(h[16:], 16)
	le.PutUint16(h[20:], 1)
	le.PutUint16(h[22:], 2)
	le.PutUint32(h[24:], uint32(rate))
	le.PutUint32(h[28:], uint32(rate*4))
	le.PutUint16(h[32:], 4)
	le.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	le.PutUint32(h[40:], uint32(len(data)))
	p := filepath.Join(dir, "tone.wav")
	if err := os.WriteFile(p, append(h, data...), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "take.wav")
	os.WriteFile(src, []byte("x"), 0o644)
	if got, _ := Target(src, "mp3"); got != filepath.Join(dir, "take.mp3") {
		t.Errorf("target %s", got)
	}
	if got, _ := Target(src, "wav16"); got != filepath.Join(dir, "take 2.wav") {
		t.Errorf("same name as the source: %s", got)
	}
}

func TestConvertFormats(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Media Foundation only")
	}
	dir := t.TempDir()
	src := tone(t, dir)
	for _, f := range []string{"mp3", "aac", "flac", "wav16", "wav24"} {
		t.Run(f, func(t *testing.T) {
			dst, _ := Target(src, f)
			last := 0.0
			err := Convert(src, dst, Options{Format: f, Progress: func(p float64) { last = p }})
			if err != nil {
				t.Fatal(err)
			}
			fi, err := os.Stat(dst)
			if err != nil || fi.Size() < 1000 {
				t.Fatalf("output %v %v", fi, err)
			}
			if last < 0.99 {
				t.Errorf("progress ended at %v", last)
			}
			t.Logf("%s: %d bytes", filepath.Base(dst), fi.Size())
			if f == "wav24" {
				data, _ := os.ReadFile(dst)
				if w, err := audio.WAVPeaks(bytes.NewReader(data), 10); err != nil || w.Duration < 0.99 || w.Duration > 1.01 {
					t.Errorf("24-bit WAV: %v %+v", err, w)
				}
			}
		})
	}
}

func TestConvertMP3BackToWAV(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Media Foundation only")
	}
	dir := t.TempDir()
	src := tone(t, dir)
	mp3, _ := Target(src, "mp3")
	if err := Convert(src, mp3, Options{Format: "mp3", Bitrate: 128}); err != nil {
		t.Fatal(err)
	}
	wav, _ := Target(mp3, "wav16")
	if err := Convert(mp3, wav, Options{Format: "wav16"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(wav)
	w, err := audio.WAVPeaks(bytes.NewReader(data), 10)
	if err != nil || w.Duration < 0.95 || w.Max[5] < 0.2 {
		t.Fatalf("decoded MP3: %v %+v", err, w)
	}
	if err := Convert(src, mp3+"x", Options{Format: "mp3", Bitrate: 111}); err == nil {
		t.Error("an odd bitrate was accepted")
	}
}

func TestPlan(t *testing.T) {
	hi := Info{Rate: 96000, Channels: 2, Bits: 24}
	r, err := Plan("mp3", hi, Options{})
	if err != nil || r.Rate != 48000 || r.Bits != 16 || len(r.Notes) != 1 {
		t.Fatalf("mp3 from 96k: %v %+v", err, r)
	}
	if r, _ := Plan("mp3", Info{Rate: 88200, Channels: 2}, Options{}); r.Rate != 44100 {
		t.Errorf("88.2k should become 44.1k, got %d", r.Rate)
	}
	if r, _ := Plan("flac", hi, Options{}); r.Rate != 96000 || r.Bits != 24 || len(r.Notes) != 0 {
		t.Errorf("flac keeps the original: %+v", r)
	}
	if r, _ := Plan("wav16", hi, Options{Rate: 44100, Channels: 1}); r.Rate != 44100 || r.Channels != 1 || r.Bits != 16 {
		t.Errorf("wav16 mono 44.1: %+v", r)
	}
	if _, err := Plan("aac", hi, Options{Rate: 96000}); err == nil {
		t.Error("AAC at 96 kHz accepted")
	}
	if r, _ := Plan("wav24", Info{Rate: 48000, Channels: 6, Bits: 24}, Options{}); r.Channels != 2 || len(r.Notes) != 1 {
		t.Errorf("6 channels: %+v", r)
	}
}

func TestConvertRateAndChannels(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Media Foundation only")
	}
	dir := t.TempDir()
	src := tone(t, dir)
	if in, err := Probe(src); err != nil || in.Rate != 44100 || in.Channels != 2 || in.Bits != 16 {
		t.Fatalf("probe: %v %+v", err, in)
	}
	// Mono at 48 kHz (resampled and mixed down by the decoder).
	dst := filepath.Join(dir, "mono48.wav")
	if err := Convert(src, dst, Options{Format: "wav16", Rate: 48000, Channels: 1}); err != nil {
		t.Fatal(err)
	}
	in, err := Probe(dst)
	if err != nil || in.Rate != 48000 || in.Channels != 1 || in.Seconds < 0.99 || in.Seconds > 1.01 {
		t.Fatalf("mono 48k: %v %+v", err, in)
	}
	// A 96 kHz original to MP3: 48 kHz; mono MP3 too.
	hi := toneAt(t, t.TempDir(), 96000)
	mp3 := filepath.Join(dir, "from96.mp3")
	if err := Convert(hi, mp3, Options{Format: "mp3"}); err != nil {
		t.Fatal(err)
	}
	if in, err := Probe(mp3); err != nil || in.Rate != 48000 {
		t.Fatalf("mp3 from 96k: %v %+v", err, in)
	}
	// Mono MP3 at 320 kbps: the encoder stops lower, so the best below it.
	r, err := Plan("mp3", Info{Rate: 44100, Channels: 2, Bits: 16}, Options{Channels: 1})
	if err != nil || r.Bitrate == 0 || r.Bitrate >= 320 || len(r.Notes) != 1 {
		t.Fatalf("mono mp3 plan: %v %+v", err, r)
	}
	for _, format := range []string{"mp3", "aac"} {
		if err := Convert(src, filepath.Join(dir, "mono."+format), Options{Format: format, Channels: 1}); err != nil {
			t.Fatalf("mono %s: %v", format, err)
		}
	}
}
