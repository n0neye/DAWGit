package preview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"path"
	"strings"
	"testing"

	"golang.org/x/image/tiff"
)

// decode runs the decoder for name's type (the pixels, before JPEG/PNG
// encoding, which blurs colors of tiny images), and checks the whole way
// gives an image too.
func decode(t *testing.T, name string, data []byte) image.Image {
	t.Helper()
	img, err := decoders[strings.ToLower(path.Ext(name))](data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	out, _, err := Image(name, bytes.NewReader(data), 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if _, _, err := image.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("%s: preview isn't an image: %v", name, err)
	}
	return img
}

func near(c color.Color, r, g, b uint8) bool {
	cr, cg, cb, _ := c.RGBA()
	d := func(x uint32, y uint8) bool { v := int(x>>8) - int(y); return v > -8 && v < 8 }
	return d(cr, r) && d(cg, g) && d(cb, b)
}

func TestTGA(t *testing.T) {
	// 2x1, top-down, 24-bit: red, blue (stored BGR).
	head := []byte{0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 1, 0, 24, 0x20}
	raw := append(append([]byte(nil), head...), 0, 0, 255, 255, 0, 0)
	img := decode(t, "a.tga", raw)
	if !near(img.At(0, 0), 255, 0, 0) || !near(img.At(1, 0), 0, 0, 255) {
		t.Fatalf("raw: %v %v", img.At(0, 0), img.At(1, 0))
	}
	rle := append([]byte(nil), head...)
	rle[2] = 10
	rle = append(rle, 0x81, 0, 255, 0) // two green pixels
	img = decode(t, "b.tga", rle)
	if !near(img.At(0, 0), 0, 255, 0) || !near(img.At(1, 0), 0, 255, 0) {
		t.Fatal("rle")
	}
}

// psd builds a 2x2 RGB Photoshop file with a raw composite.
func psd(r, g, b uint8) []byte {
	var buf bytes.Buffer
	w := func(v any) { binary.Write(&buf, binary.BigEndian, v) }
	buf.WriteString("8BPS")
	w(uint16(1))
	buf.Write(make([]byte, 6))
	w(uint16(3))
	w(uint32(2))
	w(uint32(2))
	w(uint16(8))
	w(uint16(3))
	w(uint32(0)) // color mode data
	w(uint32(0)) // resources
	w(uint32(0)) // layers
	w(uint16(0)) // raw
	for _, v := range []uint8{r, g, b} {
		buf.Write([]byte{v, v, v, 0}) // one pixel differs: not "one color"
	}
	return buf.Bytes()
}

func TestPSD(t *testing.T) {
	img := decode(t, "art.psd", psd(200, 100, 50))
	if img.Bounds().Dx() != 2 || !near(img.At(0, 0), 200, 100, 50) || !near(img.At(1, 1), 0, 0, 0) {
		t.Fatalf("composite: %v", img.At(0, 0))
	}
	if _, _, err := Image("bad.psd", bytes.NewReader([]byte("8BPS\x00\x09")), 0); err == nil {
		t.Fatal("a broken file should fail")
	}
}

func TestBlendAndEmbedded(t *testing.T) {
	// An old-style .blend: header, a TEST block (2x1 RGBA), ENDB.
	var buf bytes.Buffer
	buf.WriteString("BLENDER-v293")
	block := func(code string, body []byte) {
		buf.WriteString(code)
		binary.Write(&buf, binary.LittleEndian, uint32(len(body)))
		buf.Write(make([]byte, 8+4+4)) // pointer, sdna, count
		buf.Write(body)
	}
	body := []byte{2, 0, 0, 0, 1, 0, 0, 0, 255, 0, 0, 255, 0, 255, 0, 255}
	block("TEST", body)
	block("ENDB", nil)
	img := decode(t, "scene.blend", buf.Bytes())
	if !near(img.At(0, 0), 255, 0, 0) || !near(img.At(1, 0), 0, 255, 0) {
		t.Fatal("blend thumbnail")
	}
	// Affinity: a PNG somewhere inside.
	small := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	small.Set(1, 1, color.NRGBA{9, 9, 200, 255})
	var p bytes.Buffer
	png.Encode(&p, small)
	file := append([]byte("\x00\xffKA junk before the preview "), p.Bytes()...)
	img = decode(t, "poster.afdesign", file)
	if !near(img.At(1, 1), 9, 9, 200) {
		t.Fatal("affinity preview")
	}
}

func TestTIFFWithUnspecifiedChannel(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = 120
	}
	var buf bytes.Buffer
	tiff.Encode(&buf, src, nil) // RGBA, ExtraSamples = 2
	data := buf.Bytes()
	// Mark the 4th channel "unspecified", as Photoshop does for a mask.
	order := binary.LittleEndian
	off := int(order.Uint32(data[4:]))
	for i := 0; i < int(order.Uint16(data[off:])); i++ {
		e := off + 2 + i*12
		if order.Uint16(data[e:]) == 338 {
			order.PutUint16(data[e+8:], 0)
		}
	}
	img := decode(t, "mask.tif", data)
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0xffff {
		t.Fatal("should be shown opaque")
	}
}

func TestFitAndSupported(t *testing.T) {
	big := image.NewRGBA(image.Rect(0, 0, 400, 100))
	if b := fit(big, 200).Bounds(); b.Dx() != 200 || b.Dy() != 50 {
		t.Fatalf("fit: %v", b)
	}
	for name, ok := range map[string]bool{"a.PSD": true, "b.png": true, "c.blend": true, "d.c4d": true, "e.mb": false, "f.exr": false} {
		if Supported(name) != ok {
			t.Errorf("%s: %v", name, !ok)
		}
	}
}
