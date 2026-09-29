package audio

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

// Waveform is an overview of a sample for drawing: for each of N equal slices
// of time, the lowest and highest sample value (-1..1) over all channels.
type Waveform struct {
	Duration float64   `json:"duration"` // seconds
	Min      []float32 `json:"min"`
	Max      []float32 `json:"max"`
}

type pcmFormat struct {
	format   int // 1 PCM, 3 float
	channels int
	rate     int
	bits     int
}

// WAVPeaks reads a WAV file (PCM 8/16/24/32-bit or float, also
// WAVE_FORMAT_EXTENSIBLE) and returns an overview with n slices. It streams
// the samples, so long files are fine.
func WAVPeaks(r io.ReadSeeker, n int) (*Waveform, error) {
	var head [12]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return nil, err
	}
	if string(head[:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return nil, errors.New("not a WAV file")
	}
	var f pcmFormat
	haveFmt := false
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return nil, errors.New("WAV: no data chunk")
		}
		id, size := string(ch[:4]), int64(binary.LittleEndian.Uint32(ch[4:8]))
		switch id {
		case "fmt ":
			b := make([]byte, size)
			if _, err := io.ReadFull(r, b); err != nil || len(b) < 16 {
				return nil, errors.New("WAV: bad fmt chunk")
			}
			le := binary.LittleEndian
			f = pcmFormat{format: int(le.Uint16(b[0:2])), channels: int(le.Uint16(b[2:4])),
				rate: int(le.Uint32(b[4:8])), bits: int(le.Uint16(b[14:16]))}
			if f.format == 0xFFFE && len(b) >= 26 { // extensible: the real format is in the sub-format GUID
				f.format = int(le.Uint16(b[24:26]))
			}
			haveFmt = true
			if size%2 == 1 {
				r.Seek(1, io.SeekCurrent)
			}
		case "data":
			if !haveFmt {
				return nil, errors.New("WAV: data before fmt")
			}
			return peaks(io.LimitReader(r, size), f, size, n)
		default:
			if _, err := r.Seek(size+size%2, io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}
}

// AIFFPeaks is WAVPeaks for AIFF / AIFF-C (uncompressed); data is the whole file.
func AIFFPeaks(data []byte, n int) (*Waveform, error) {
	wav, err := AIFFToWAV(data)
	if err != nil {
		return nil, err
	}
	return WAVPeaks(bytes.NewReader(wav), n)
}

func peaks(r io.Reader, f pcmFormat, size int64, n int) (*Waveform, error) {
	if f.channels < 1 || f.rate < 1 || n < 1 {
		return nil, errors.New("audio: bad format")
	}
	if f.format != 1 && f.format != 3 {
		return nil, fmt.Errorf("audio: format %d is not supported", f.format)
	}
	bytesPer := (f.bits + 7) / 8
	if bytesPer < 1 || bytesPer > 8 || (f.format == 3 && bytesPer != 4 && bytesPer != 8) {
		return nil, fmt.Errorf("audio: %d-bit samples are not supported", f.bits)
	}
	frameSize := int64(bytesPer * f.channels)
	frames := size / frameSize
	w := &Waveform{Duration: float64(frames) / float64(f.rate), Min: make([]float32, n), Max: make([]float32, n)}
	if frames == 0 {
		return w, nil
	}
	sample := decoder(f, bytesPer)
	br := bufio.NewReaderSize(r, 1<<16)
	buf := make([]byte, frameSize)
	for i := int64(0); i < frames; i++ {
		if _, err := io.ReadFull(br, buf); err != nil {
			break // truncated: draw what is there
		}
		slice := int(i * int64(n) / frames)
		for c := 0; c < f.channels; c++ {
			v := sample(buf[c*bytesPer : (c+1)*bytesPer])
			if v < w.Min[slice] {
				w.Min[slice] = v
			}
			if v > w.Max[slice] {
				w.Max[slice] = v
			}
		}
	}
	return w, nil
}

// decoder turns one little-endian sample's bytes into -1..1.
func decoder(f pcmFormat, bytesPer int) func([]byte) float32 {
	order := binary.LittleEndian
	if f.format == 3 {
		if bytesPer == 8 {
			return func(b []byte) float32 { return float32(math.Float64frombits(order.Uint64(b))) }
		}
		return func(b []byte) float32 { return math.Float32frombits(order.Uint32(b)) }
	}
	if bytesPer == 1 { // WAV 8-bit is unsigned
		return func(b []byte) float32 { return (float32(b[0]) - 128) / 128 }
	}
	scale := float32(math.Pow(2, float64(bytesPer*8-1)))
	return func(b []byte) float32 {
		var v int64
		for k := len(b) - 1; k >= 0; k-- {
			v = v<<8 | int64(b[k])
		}
		shift := 64 - 8*len(b) // sign-extend
		v = v << shift >> shift
		return float32(v) / scale
	}
}
