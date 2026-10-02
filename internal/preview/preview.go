// Package preview turns design files into images the app can show: formats
// the web view reads (PNG, JPEG, GIF, WebP, BMP, SVG) pass through; others
// are decoded (Photoshop's composite, TIFF, TGA) or give the preview their
// program saves inside them (Affinity, Blender, Cinema 4D).
package preview

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"path"
	"strings"

	"golang.org/x/image/draw"
)

// ErrNoPreview: the file's type has no preview, or this file has none.
var ErrNoPreview = errors.New("no preview")

// maxRead bounds the bytes read from one file.
const maxRead = 1 << 30

// native are the image types the web view shows as they are.
var native = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".bmp": "image/bmp", ".svg": "image/svg+xml", ".ico": "image/x-icon", ".avif": "image/avif",
}

// IsVideo: a video the page can try to play (H.264 or VP8/9 inside; other
// codecs, ProRes say, it can't).
func IsVideo(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".mp4", ".m4v", ".mov", ".webm":
		return true
	}
	return false
}

// IsModel: a 3D model the page can show (glTF, FBX, OBJ, STL, PLY).
func IsModel(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".glb", ".gltf", ".fbx", ".obj", ".stl", ".ply":
		return true
	}
	return false
}

// Supported reports whether files named so can have a preview.
func Supported(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	if _, ok := native[ext]; ok {
		return true
	}
	_, ok := decoders[ext]
	return ok
}

// decoders read an image (or a file's own preview) from the whole file.
var decoders = map[string]func([]byte) (image.Image, error){
	".psd":      decodePSD,
	".psb":      decodePSD,
	".tif":      decodeTIFF,
	".tiff":     decodeTIFF,
	".tga":      decodeTGA,
	".af":       embeddedPNG, // Affinity's one format (from Affinity 3)
	".afphoto":  embeddedPNG,
	".afdesign": embeddedPNG,
	".afpub":    embeddedPNG,
	".blend":    decodeBlend,
	".c4d":      embeddedJPEG,
}

// Image returns a preview of the file named name (by its type) read from r,
// at most maxSide pixels on its longer side when it has to be decoded
// (native images are passed through as they are), and its content type.
func Image(name string, r io.Reader, maxSide int) ([]byte, string, error) {
	ext := strings.ToLower(path.Ext(name))
	data, err := io.ReadAll(io.LimitReader(r, maxRead))
	if err != nil {
		return nil, "", err
	}
	if t, ok := native[ext]; ok {
		return data, t, nil
	}
	dec, ok := decoders[ext]
	if !ok {
		return nil, "", ErrNoPreview
	}
	img, err := dec(data)
	if err != nil {
		return nil, "", err
	}
	return encode(fit(img, maxSide))
}

// fit scales img down to maxSide pixels on its longer side.
func fit(img image.Image, maxSide int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxSide <= 0 || (w <= maxSide && h <= maxSide) {
		return img
	}
	if w >= h {
		w, h = maxSide, max(1, h*maxSide/w)
	} else {
		w, h = max(1, w*maxSide/h), maxSide
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// encode writes JPEG for opaque images (smaller, quicker), PNG otherwise.
func encode(img image.Image) ([]byte, string, error) {
	var buf bytes.Buffer
	if opaque(img) {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/jpeg", nil
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/png", nil
}

func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return false
}
