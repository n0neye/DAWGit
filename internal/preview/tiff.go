package preview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"strings"

	"golang.org/x/image/tiff"
)

// decodeTIFF reads TIFF files. Photoshop saves a 4th channel (an alpha
// channel or mask) marked "unspecified", which the decoder refuses: such
// files are read with that channel taken as alpha, then shown opaque.
func decodeTIFF(data []byte) (image.Image, error) {
	img, err := tiff.Decode(bytes.NewReader(data))
	if err == nil || !strings.Contains(err.Error(), "wrong number of samples") {
		return img, err
	}
	patched, ok := extraSamplesAsAlpha(data)
	if !ok {
		return nil, err
	}
	img, err2 := tiff.Decode(bytes.NewReader(patched))
	if err2 != nil {
		return nil, err
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	for i := 3; i < len(out.Pix); i += 4 {
		out.Pix[i] = 255
	}
	return out, nil
}

// extraSamplesAsAlpha copies data with the first image's ExtraSamples tag
// (338) set to 2, "unassociated alpha".
func extraSamplesAsAlpha(data []byte) ([]byte, bool) {
	if len(data) < 8 {
		return nil, false
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, false
	}
	off := int(order.Uint32(data[4:]))
	if off+2 > len(data) {
		return nil, false
	}
	n := int(order.Uint16(data[off:]))
	for i := 0; i < n; i++ {
		e := off + 2 + i*12
		if e+12 > len(data) {
			return nil, false
		}
		if order.Uint16(data[e:]) != 338 || order.Uint16(data[e+2:]) != 3 || order.Uint32(data[e+4:]) != 1 {
			continue
		}
		out := append([]byte(nil), data...)
		order.PutUint16(out[e+8:], 2)
		return out, true
	}
	return nil, false
}
