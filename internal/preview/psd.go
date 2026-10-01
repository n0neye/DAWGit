package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"math"
)

// Photoshop files (PSD, and PSB for big ones) keep a composite of all
// layers after them ("Maximize compatibility", on by default), and a small
// JPEG thumbnail among their resources: the composite, or else the
// thumbnail.

var errPSD = errors.New("not a Photoshop file this can read")

// maxPixels bounds a composite decoded whole (8K x 8K is 67M).
const maxPixels = 100_000_000

type reader struct {
	b   []byte
	off int
	bad bool
}

func (r *reader) take(n int) []byte {
	if n < 0 || r.off+n > len(r.b) {
		r.bad = true
		r.off = len(r.b)
		return nil
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out
}

func (r *reader) u16() int {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return int(binary.BigEndian.Uint16(b))
}

func (r *reader) u32() int {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return int(binary.BigEndian.Uint32(b))
}

func (r *reader) u64() int {
	b := r.take(8)
	if b == nil {
		return 0
	}
	return int(binary.BigEndian.Uint64(b))
}

func decodePSD(data []byte) (image.Image, error) {
	r := &reader{b: data}
	if string(r.take(4)) != "8BPS" {
		return nil, errPSD
	}
	ver := r.u16()
	if ver != 1 && ver != 2 {
		return nil, errPSD
	}
	r.take(6)
	channels, h, w, depth, mode := r.u16(), r.u32(), r.u32(), r.u16(), r.u16()
	r.take(r.u32()) // color mode data
	resources := r.take(r.u32())
	if ver == 1 {
		r.take(r.u32()) // layers
	} else {
		r.take(r.u64())
	}
	if r.bad {
		return nil, errPSD
	}
	img, err := psdComposite(r, ver, channels, w, h, depth, mode)
	if err == nil && !uniform(img) {
		return img, nil
	}
	if thumb := psdThumbnail(resources); thumb != nil {
		return thumb, nil
	}
	if err != nil {
		return nil, err
	}
	return img, nil // one color after all
}

func psdComposite(r *reader, ver, channels, w, h, depth, mode int) (image.Image, error) {
	need := map[int]int{3: 3, 1: 1, 4: 4}[mode] // RGB, grayscale, CMYK
	if need == 0 || channels < need || w <= 0 || h <= 0 || w*h > maxPixels {
		return nil, errPSD
	}
	if depth != 8 && depth != 16 && depth != 32 {
		return nil, errPSD
	}
	bps := depth / 8
	rowBytes := w * bps
	compression := r.u16()
	planes := make([][]byte, need)
	switch compression {
	case 0: // raw
		for c := 0; c < need; c++ {
			planes[c] = r.take(rowBytes * h)
		}
	case 1: // PackBits, row by row, after every row's length
		countSize := 2
		if ver == 2 {
			countSize = 4
		}
		counts := r.take(channels * h * countSize)
		for c := 0; c < need && !r.bad; c++ {
			plane := make([]byte, 0, rowBytes*h)
			for y := 0; y < h; y++ {
				i := (c*h + y) * countSize
				n := int(binary.BigEndian.Uint16(counts[i:]))
				if countSize == 4 {
					n = int(binary.BigEndian.Uint32(counts[i:]))
				}
				row, ok := unpackBits(r.take(n), rowBytes)
				if !ok {
					return nil, errPSD
				}
				plane = append(plane, row...)
			}
			planes[c] = plane
		}
	default: // ZIP: rare for the composite
		return nil, errPSD
	}
	if r.bad {
		return nil, errPSD
	}
	sample := func(c, i int) uint8 {
		p := planes[c][i*bps:]
		switch bps {
		case 1:
			return p[0]
		case 2:
			return p[0]
		}
		f := math.Float32frombits(binary.BigEndian.Uint32(p))
		return uint8(math.Max(0, math.Min(1, math.Pow(float64(f), 1/2.2))) * 255)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		var c color.RGBA
		switch mode {
		case 3:
			c = color.RGBA{sample(0, i), sample(1, i), sample(2, i), 255}
		case 1:
			v := sample(0, i)
			c = color.RGBA{v, v, v, 255}
		case 4: // stored inverted: 255 is no ink
			k := int(sample(3, i))
			c = color.RGBA{uint8(int(sample(0, i)) * k / 255), uint8(int(sample(1, i)) * k / 255),
				uint8(int(sample(2, i)) * k / 255), 255}
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = c.R, c.G, c.B, c.A
	}
	return img, nil
}

// unpackBits decodes one PackBits row of n bytes.
func unpackBits(src []byte, n int) ([]byte, bool) {
	out := make([]byte, 0, n)
	for i := 0; i < len(src) && len(out) < n; {
		h := int(int8(src[i]))
		i++
		switch {
		case h >= 0:
			if i+h+1 > len(src) {
				return nil, false
			}
			out = append(out, src[i:i+h+1]...)
			i += h + 1
		case h != -128:
			if i >= len(src) {
				return nil, false
			}
			out = append(out, bytes.Repeat([]byte{src[i]}, 1-h)...)
			i++
		}
	}
	if len(out) < n {
		return nil, false
	}
	return out[:n], true
}

// psdThumbnail reads resource 1036 (a JPEG thumbnail).
func psdThumbnail(res []byte) image.Image {
	r := &reader{b: res}
	for r.off < len(res) && !r.bad {
		if string(r.take(4)) != "8BIM" {
			return nil
		}
		id := r.u16()
		nameLen := 0
		if b := r.take(1); b != nil {
			nameLen = int(b[0])
		}
		r.take(nameLen + (nameLen+1)%2) // a Pascal string, padded to even
		size := r.u32()
		data := r.take(size)
		if size%2 == 1 {
			r.take(1)
		}
		if id == 1036 && len(data) > 28 {
			img, err := jpeg.Decode(bytes.NewReader(data[28:]))
			if err == nil {
				return img
			}
		}
	}
	return nil
}

// uniform: one color everywhere (sampled) — a composite Photoshop didn't
// write.
func uniform(img image.Image) bool {
	b := img.Bounds()
	first := img.At(b.Min.X, b.Min.Y)
	for y := b.Min.Y; y < b.Max.Y; y += max(1, b.Dy()/32) {
		for x := b.Min.X; x < b.Max.X; x += max(1, b.Dx()/32) {
			if img.At(x, y) != first {
				return false
			}
		}
	}
	return true
}
