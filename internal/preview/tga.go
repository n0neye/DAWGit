package preview

import (
	"errors"
	"image"
)

var errTGA = errors.New("not a TGA file this can read")

// decodeTGA reads true-color and grayscale TGA files, plain or run-length
// encoded, 8, 24 or 32 bits a pixel.
func decodeTGA(data []byte) (image.Image, error) {
	if len(data) < 18 {
		return nil, errTGA
	}
	idLen, cmapType, kind := int(data[0]), data[1], data[2]
	w, h := int(data[12])|int(data[13])<<8, int(data[14])|int(data[15])<<8
	bpp, desc := int(data[16]), data[17]
	gray := kind == 3 || kind == 11
	rle := kind == 10 || kind == 11
	if cmapType != 0 || !(kind == 2 || kind == 3 || kind == 10 || kind == 11) || w == 0 || h == 0 || w*h > maxPixels {
		return nil, errTGA
	}
	if (gray && bpp != 8) || (!gray && bpp != 24 && bpp != 32) {
		return nil, errTGA
	}
	px := bpp / 8
	src := data[18+idLen:]
	raw := make([]byte, 0, w*h*px)
	if !rle {
		if len(src) < w*h*px {
			return nil, errTGA
		}
		raw = src[:w*h*px]
	} else {
		for i := 0; len(raw) < w*h*px; {
			if i >= len(src) {
				return nil, errTGA
			}
			head := int(src[i])
			i++
			n := head&0x7f + 1
			if head&0x80 != 0 { // one pixel, repeated
				if i+px > len(src) {
					return nil, errTGA
				}
				for k := 0; k < n; k++ {
					raw = append(raw, src[i:i+px]...)
				}
				i += px
			} else {
				if i+n*px > len(src) {
					return nil, errTGA
				}
				raw = append(raw, src[i:i+n*px]...)
				i += n * px
			}
		}
		raw = raw[:w*h*px]
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	topDown := desc&0x20 != 0
	alpha := bpp == 32 && desc&0x0f != 0
	for y := 0; y < h; y++ {
		row := y
		if !topDown {
			row = h - 1 - y
		}
		for x := 0; x < w; x++ {
			s := raw[(row*w+x)*px:]
			d := img.Pix[(y*w+x)*4:]
			if gray {
				d[0], d[1], d[2], d[3] = s[0], s[0], s[0], 255
				continue
			}
			d[0], d[1], d[2], d[3] = s[2], s[1], s[0], 255 // stored BGR(A)
			if alpha {
				d[3] = s[3]
			}
		}
	}
	return img, nil
}
