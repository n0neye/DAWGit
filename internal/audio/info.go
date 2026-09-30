package audio

import (
	"encoding/binary"
	"errors"
	"io"
)

// Info is a PCM file's format from its header.
type Info struct {
	Rate, Channels, Bits int
	Seconds              float64
}

// WAVInfo reads a WAV file's format and length from its chunks.
func WAVInfo(r io.ReadSeeker) (Info, error) {
	var head [12]byte
	if _, err := io.ReadFull(r, head[:]); err != nil || string(head[:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return Info{}, errors.New("not a WAV file")
	}
	var in Info
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return Info{}, errors.New("WAV: no data chunk")
		}
		id, size := string(ch[:4]), int64(binary.LittleEndian.Uint32(ch[4:]))
		switch id {
		case "fmt ":
			b := make([]byte, size)
			if _, err := io.ReadFull(r, b); err != nil || len(b) < 16 {
				return Info{}, errors.New("WAV: bad fmt chunk")
			}
			le := binary.LittleEndian
			in = Info{Channels: int(le.Uint16(b[2:])), Rate: int(le.Uint32(b[4:])), Bits: int(le.Uint16(b[14:]))}
			if size%2 == 1 {
				r.Seek(1, io.SeekCurrent)
			}
		case "data":
			if frame := in.Channels * ((in.Bits + 7) / 8); frame > 0 && in.Rate > 0 {
				in.Seconds = float64(size/int64(frame)) / float64(in.Rate)
			}
			return in, nil
		default:
			if _, err := r.Seek(size+size%2, io.SeekCurrent); err != nil {
				return Info{}, err
			}
		}
	}
}

// AIFFInfo reads an AIFF / AIFF-C file's format and length (COMM chunk).
func AIFFInfo(r io.ReadSeeker) (Info, error) {
	var head [12]byte
	if _, err := io.ReadFull(r, head[:]); err != nil || !IsAIFF(head[:]) {
		return Info{}, errors.New("not an AIFF file")
	}
	for {
		var ch [8]byte
		if _, err := io.ReadFull(r, ch[:]); err != nil {
			return Info{}, errors.New("AIFF: no COMM chunk")
		}
		id, size := string(ch[:4]), int64(binary.BigEndian.Uint32(ch[4:]))
		if id != "COMM" {
			if _, err := r.Seek(size+size%2, io.SeekCurrent); err != nil {
				return Info{}, err
			}
			continue
		}
		b := make([]byte, size)
		if _, err := io.ReadFull(r, b); err != nil || len(b) < 18 {
			return Info{}, errors.New("AIFF: bad COMM chunk")
		}
		be := binary.BigEndian
		in := Info{Channels: int(be.Uint16(b[0:])), Bits: int(be.Uint16(b[6:])), Rate: int(extended(b[8:18]) + .5)}
		if in.Rate > 0 {
			in.Seconds = float64(be.Uint32(b[2:])) / float64(in.Rate)
		}
		return in, nil
	}
}
