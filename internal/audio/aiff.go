// Package audio converts AIFF to WAV, so the app's web view (which plays WAV,
// MP3, FLAC and OGG but not AIFF) can preview it.
package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// IsAIFF reports whether data starts like an AIFF or AIFF-C file.
func IsAIFF(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "FORM" && (string(data[8:12]) == "AIFF" || string(data[8:12]) == "AIFC")
}

// AIFFToWAV converts uncompressed AIFF / AIFF-C ("NONE", "sowt", "fl32",
// "fl64") to a WAV file.
func AIFFToWAV(data []byte) ([]byte, error) {
	if !IsAIFF(data) {
		return nil, errors.New("not an AIFF file")
	}
	var (
		channels, bits int
		rate           float64
		compression    = "NONE"
		sound          []byte
		haveComm       bool
	)
	for p := 12; p+8 <= len(data); {
		id := string(data[p : p+4])
		size := int(binary.BigEndian.Uint32(data[p+4 : p+8]))
		body := data[p+8:]
		if size > len(body) {
			size = len(body) // truncated file: use what is there
		}
		body = body[:size]
		switch id {
		case "COMM":
			if len(body) < 18 {
				return nil, errors.New("AIFF: short COMM chunk")
			}
			channels = int(binary.BigEndian.Uint16(body[0:2]))
			bits = int(binary.BigEndian.Uint16(body[6:8]))
			rate = extended(body[8:18])
			if string(data[8:12]) == "AIFC" && len(body) >= 22 {
				compression = string(body[18:22])
			}
			haveComm = true
		case "SSND":
			if len(body) < 8 {
				return nil, errors.New("AIFF: short SSND chunk")
			}
			offset := int(binary.BigEndian.Uint32(body[0:4]))
			if 8+offset > len(body) {
				return nil, errors.New("AIFF: bad SSND offset")
			}
			sound = body[8+offset:]
		}
		p += 8 + size + size%2 // chunks are padded to an even length
	}
	if !haveComm || sound == nil {
		return nil, errors.New("AIFF: missing COMM or SSND chunk")
	}
	if channels < 1 || rate <= 0 {
		return nil, errors.New("AIFF: bad format")
	}

	format := 1 // PCM
	bigEndian := true
	switch compression {
	case "NONE", "twos":
	case "sowt":
		bigEndian = false
	case "fl32", "FL32":
		format, bits = 3, 32
	case "fl64", "FL64":
		format, bits = 3, 64
	default:
		return nil, fmt.Errorf("AIFF-C compression %q is not supported", compression)
	}
	if bits < 8 || bits > 64 {
		return nil, fmt.Errorf("AIFF: %d-bit samples are not supported", bits)
	}
	bytesPer := (bits + 7) / 8
	frame := bytesPer * channels
	sound = sound[:len(sound)-len(sound)%frame]

	pcm := make([]byte, len(sound))
	copy(pcm, sound)
	switch {
	case bytesPer == 1:
		for i := range pcm {
			pcm[i] += 128 // AIFF 8-bit is signed, WAV 8-bit unsigned
		}
	case bigEndian:
		for i := 0; i < len(pcm); i += bytesPer {
			s := pcm[i : i+bytesPer]
			for a, b := 0, len(s)-1; a < b; a, b = a+1, b-1 {
				s[a], s[b] = s[b], s[a]
			}
		}
	}

	var out bytes.Buffer
	w := func(v any) { binary.Write(&out, binary.LittleEndian, v) }
	out.WriteString("RIFF")
	w(uint32(36 + len(pcm)))
	out.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(format))
	w(uint16(channels))
	w(uint32(math.Round(rate)))
	w(uint32(int(math.Round(rate)) * frame))
	w(uint16(frame))
	w(uint16(bytesPer * 8))
	out.WriteString("data")
	w(uint32(len(pcm)))
	out.Write(pcm)
	return out.Bytes(), nil
}

// extended decodes an 80-bit IEEE 754 extended float (AIFF sample rates).
func extended(b []byte) float64 {
	exp := int(binary.BigEndian.Uint16(b[0:2]) & 0x7fff)
	mant := binary.BigEndian.Uint64(b[2:10])
	if exp == 0 && mant == 0 {
		return 0
	}
	v := float64(mant) * math.Pow(2, float64(exp-16383-63))
	if b[0]&0x80 != 0 {
		v = -v
	}
	return v
}
