// Package convert turns a sample into another audio format with the
// system's own codecs (Windows: Media Foundation). Nothing is downloaded.
package convert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Format is a target format.
type Format struct {
	ID      string // mp3 | aac | flac | wav16 | wav24
	Name    string // shown to the user
	Ext     string // file extension, with the dot
	Bitrate []int  // kbps choices for lossy formats (first is the default)
}

// Formats lists what Convert can write, lossy formats first.
var Formats = []Format{
	{ID: "mp3", Name: "MP3", Ext: ".mp3", Bitrate: []int{320, 256, 192, 128}},
	{ID: "aac", Name: "AAC (.m4a)", Ext: ".m4a", Bitrate: []int{192, 160, 128, 96}},
	{ID: "flac", Name: "FLAC (lossless)", Ext: ".flac"},
	{ID: "wav16", Name: "WAV 16-bit", Ext: ".wav"},
	{ID: "wav24", Name: "WAV 24-bit", Ext: ".wav"},
}

func formatByID(id string) (Format, bool) {
	for _, f := range Formats {
		if f.ID == id {
			return f, true
		}
	}
	return Format{}, false
}

// Options for a conversion.
type Options struct {
	Format  string // a Format ID
	Bitrate int    // kbps, for lossy formats (0: the default)
	// Progress, when set, hears how far along it is (0..1).
	Progress func(done float64)
}

// ErrUnsupported: this system has no codec for the conversion.
var ErrUnsupported = errors.New("converting audio is not available on this system yet")

// Target is where a conversion of src to format goes: next to it, with the
// new extension; " 2", " 3"… when that name is taken (or is src itself).
func Target(src, format string) (string, error) {
	f, ok := formatByID(format)
	if !ok {
		return "", fmt.Errorf("unknown format %q", format)
	}
	base := strings.TrimSuffix(src, filepath.Ext(src))
	dst := base + f.Ext
	for n := 2; ; n++ {
		if _, err := os.Stat(dst); os.IsNotExist(err) && !strings.EqualFold(dst, src) {
			return dst, nil
		}
		dst = fmt.Sprintf("%s %d%s", base, n, f.Ext)
	}
}

// Convert writes src as dst in the chosen format.
func Convert(src, dst string, o Options) error {
	f, ok := formatByID(o.Format)
	if !ok {
		return fmt.Errorf("unknown format %q", o.Format)
	}
	if o.Bitrate == 0 && len(f.Bitrate) > 0 {
		o.Bitrate = f.Bitrate[0]
	}
	if len(f.Bitrate) > 0 && !contains(f.Bitrate, o.Bitrate) {
		return fmt.Errorf("%s does not come at %d kbps", f.Name, o.Bitrate)
	}
	if o.Progress == nil {
		o.Progress = func(float64) {}
	}
	// The encoder picks the container from the extension: keep it.
	tmp := strings.TrimSuffix(dst, f.Ext) + ".part" + f.Ext
	os.Remove(tmp)
	if err := convert(src, tmp, f, o); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
