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
func tone(t *testing.T, dir string) string {
	t.Helper()
	const rate, frames = 44100, 44100
	data := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		v := int16(12000 * math.Sin(2*math.Pi*440*float64(i)/rate))
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
	le.PutUint32(h[24:], rate)
	le.PutUint32(h[28:], rate*4)
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
