package preview

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"strconv"
)

// Previews programs keep inside their files.

// embeddedPNG: Affinity files keep a PNG preview near their start.
func embeddedPNG(data []byte) (image.Image, error) {
	i := bytes.Index(data, []byte("\x89PNG\r\n\x1a\n"))
	if i < 0 {
		return nil, ErrNoPreview
	}
	return png.Decode(bytes.NewReader(data[i:]))
}

// embeddedJPEG: Cinema 4D files keep a JPEG preview near their start.
func embeddedJPEG(data []byte) (image.Image, error) {
	i := bytes.Index(data[:min(len(data), 1<<20)], []byte{0xff, 0xd8, 0xff})
	if i < 0 {
		return nil, ErrNoPreview
	}
	return jpeg.Decode(bytes.NewReader(data[i:]))
}

var errBlend = errors.New("no preview in this .blend")

// decodeBlend reads the thumbnail Blender saves in a .blend (a "TEST"
// block: width, height, then RGBA rows from the bottom). Compressed .blend
// files (an option since Blender 3) have no preview here.
func decodeBlend(data []byte) (image.Image, error) {
	if !bytes.HasPrefix(data, []byte("BLENDER")) {
		return nil, errBlend
	}
	// Blender 5: "BLENDER17-01v0500"; before: "BLENDER-v293" (pointer size
	// '-' 8 or '_' 4, then 'v' little or 'V' big endian).
	var ptr, off int
	var order binary.ByteOrder = binary.LittleEndian
	wide := false
	if len(data) > 9 && data[7] >= '0' && data[7] <= '9' {
		n, err := strconv.Atoi(string(data[7:9]))
		if err != nil || n > len(data) {
			return nil, errBlend
		}
		ptr, off, wide = 8, n, true
		if bytes.IndexByte(data[:n], 'V') >= 0 {
			order = binary.BigEndian
		}
	} else if len(data) >= 12 {
		ptr, off = 4, 12
		if data[7] == '-' {
			ptr = 8
		}
		if data[8] == 'V' {
			order = binary.BigEndian
		}
	} else {
		return nil, errBlend
	}
	for off+8 < len(data) {
		code := string(data[off : off+4])
		var size, head int
		if wide { // code, sdna, pointer, length (8), count (8)
			head = 4 + 4 + 8 + 8 + 8
			if off+head > len(data) {
				break
			}
			size = int(order.Uint64(data[off+16:]))
		} else { // code, length, pointer, sdna, count
			head = 4 + 4 + ptr + 4 + 4
			if off+head > len(data) {
				break
			}
			size = int(order.Uint32(data[off+4:]))
		}
		body := off + head
		if size < 0 || body+size > len(data) {
			break
		}
		if code == "TEST" && size > 8 {
			w, h := int(order.Uint32(data[body:])), int(order.Uint32(data[body+4:]))
			if w <= 0 || h <= 0 || 8+w*h*4 > size {
				return nil, errBlend
			}
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			pix := data[body+8:]
			for y := 0; y < h; y++ {
				copy(img.Pix[y*w*4:(y+1)*w*4], pix[(h-1-y)*w*4:(h-y)*w*4])
			}
			return img, nil
		}
		if code == "ENDB" || code == "DNA1" {
			break
		}
		off = body + size
	}
	return nil, errBlend
}
