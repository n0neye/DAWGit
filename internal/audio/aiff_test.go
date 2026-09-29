package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// aiff builds a small AIFF file: 16-bit stereo at 44.1 kHz, big endian.
func aiff(samples []int16) []byte {
	var comm, ssnd, out bytes.Buffer
	be := func(b *bytes.Buffer, v any) { binary.Write(b, binary.BigEndian, v) }
	be(&comm, uint16(2))                       // channels
	be(&comm, uint32(len(samples)/2))          // frames
	be(&comm, uint16(16))                      // bits
	comm.Write([]byte{0x40, 0x0e, 0xac, 0x44}) // 44100 as 80-bit extended
	comm.Write(make([]byte, 6))
	be(&ssnd, uint32(0))
	be(&ssnd, uint32(0))
	for _, s := range samples {
		be(&ssnd, s)
	}
	out.WriteString("FORM")
	be(&out, uint32(4+8+comm.Len()+8+ssnd.Len()))
	out.WriteString("AIFF")
	out.WriteString("COMM")
	be(&out, uint32(comm.Len()))
	out.Write(comm.Bytes())
	out.WriteString("SSND")
	be(&out, uint32(ssnd.Len()))
	out.Write(ssnd.Bytes())
	return out.Bytes()
}

func TestAIFFToWAV(t *testing.T) {
	in := []int16{1, -2, 300, -32768}
	wav, err := AIFFToWAV(aiff(in))
	if err != nil {
		t.Fatal(err)
	}
	le := binary.LittleEndian
	if string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " {
		t.Fatalf("header %q", wav[:16])
	}
	if ch, rate, bits := le.Uint16(wav[22:24]), le.Uint32(wav[24:28]), le.Uint16(wav[34:36]); ch != 2 || rate != 44100 || bits != 16 {
		t.Fatalf("format: %d ch, %d Hz, %d bit", ch, rate, bits)
	}
	data := wav[44:]
	for i, want := range in {
		if got := int16(le.Uint16(data[2*i:])); got != want {
			t.Errorf("sample %d = %d, want %d", i, got, want)
		}
	}
	if _, err := AIFFToWAV([]byte("RIFF....WAVE")); err == nil {
		t.Error("a WAV file was accepted as AIFF")
	}
}

func TestExtended(t *testing.T) {
	for _, rate := range []float64{44100, 48000, 96000, 22050} {
		exp := int(math.Floor(math.Log2(rate)))
		mant := uint64(rate * math.Pow(2, float64(63-exp)))
		b := make([]byte, 10)
		binary.BigEndian.PutUint16(b, uint16(exp+16383))
		binary.BigEndian.PutUint64(b[2:], mant)
		if got := extended(b); got != rate {
			t.Errorf("extended(%v) = %v", rate, got)
		}
	}
}
