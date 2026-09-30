// Package convert turns a sample into another audio format with the
// system's own codecs (Windows: Media Foundation). Nothing is downloaded.
package convert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dawgit/internal/audio"
)

// Format is a target format.
type Format struct {
	ID      string // mp3 | aac | flac | wav16 | wav24
	Name    string // shown to the user
	Ext     string // file extension, with the dot
	Bitrate []int  // kbps choices for lossy formats (first is the default)
	Rates   []int  // sample rates the encoder takes (Hz)
	Bits    []int  // bit depths to choose from (lossless formats with a choice)
}

var (
	lossyRates    = []int{44100, 48000}
	losslessRates = []int{22050, 32000, 44100, 48000, 88200, 96000, 176400, 192000}
)

// Formats lists what Convert can write, lossy formats first.
var Formats = []Format{
	{ID: "mp3", Name: "MP3", Ext: ".mp3", Bitrate: []int{320, 256, 192, 128}, Rates: []int{32000, 44100, 48000}},
	{ID: "aac", Name: "AAC (.m4a)", Ext: ".m4a", Bitrate: []int{192, 160, 128, 96}, Rates: lossyRates},
	{ID: "flac", Name: "FLAC (lossless)", Ext: ".flac", Rates: losslessRates, Bits: []int{16, 24}},
	{ID: "wav16", Name: "WAV 16-bit", Ext: ".wav", Rates: losslessRates},
	{ID: "wav24", Name: "WAV 24-bit", Ext: ".wav", Rates: losslessRates},
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
	// 0 keeps the original's (when the format allows it).
	Rate     int // Hz
	Channels int // 1 (mono: channels mixed) or 2
	Bits     int // FLAC: 16 or 24
	// Progress, when set, hears how far along it is (0..1).
	Progress func(done float64)
}

// Info describes a sample.
type Info struct {
	Rate     int     `json:"rate"`
	Channels int     `json:"channels"`
	Bits     int     `json:"bits"` // 0 for compressed formats
	Seconds  float64 `json:"seconds"`
}

// Result is what a conversion will write, with notes on what the format
// forces (e.g. "MP3 goes up to 48 kHz: 96 kHz becomes 48 kHz").
type Result struct {
	Rate     int      `json:"rate"`
	Channels int      `json:"channels"`
	Bits     int      `json:"bits"`
	Bitrate  int      `json:"bitrate"` // kbps, for lossy formats
	Notes    []string `json:"notes"`
}

type pcm struct{ rate, channels, bits uint32 }

func khz(hz int) string {
	if hz%1000 == 0 {
		return fmt.Sprintf("%d kHz", hz/1000)
	}
	return fmt.Sprintf("%.1f kHz", float64(hz)/1000)
}

func chName(n int) string {
	if n == 1 {
		return "mono"
	}
	return "stereo"
}

// Plan works out the output for a format, the original and the options.
func Plan(format string, src Info, o Options) (Result, error) {
	f, ok := formatByID(format)
	if !ok {
		return Result{}, fmt.Errorf("unknown format %q", format)
	}
	r := Result{Notes: []string{}}

	// Sample rate: the chosen one, else the original's, else the closest the
	// encoder takes (the same family: 44.1 kHz multiples stay 44.1).
	switch {
	case o.Rate != 0:
		if !contains(f.Rates, o.Rate) {
			return Result{}, fmt.Errorf("%s does not come at %s", f.Name, khz(o.Rate))
		}
		r.Rate = o.Rate
	case src.Rate == 0:
		r.Rate = 44100
	case contains(f.Rates, src.Rate):
		r.Rate = src.Rate
	default:
		r.Rate = closestRate(f.Rates, src.Rate)
		r.Notes = append(r.Notes, fmt.Sprintf("%s takes %s at most here: %s becomes %s",
			f.Name, khz(f.Rates[len(f.Rates)-1]), khz(src.Rate), khz(r.Rate)))
	}

	switch {
	case o.Channels == 1 || o.Channels == 2:
		r.Channels = o.Channels
	case o.Channels != 0:
		return Result{}, fmt.Errorf("channels: 1 or 2")
	case src.Channels == 1:
		r.Channels = 1
	default:
		r.Channels = 2
		if src.Channels > 2 {
			r.Notes = append(r.Notes, fmt.Sprintf("%d channels are mixed down to stereo", src.Channels))
		}
	}

	switch f.ID {
	case "wav16":
		r.Bits = 16
	case "wav24":
		r.Bits = 24
	case "flac":
		switch {
		case o.Bits == 16 || o.Bits == 24:
			r.Bits = o.Bits
		case o.Bits != 0:
			return Result{}, fmt.Errorf("FLAC: 16 or 24 bits")
		case src.Bits > 16:
			r.Bits = 24
		default:
			r.Bits = 16
		}
	default:
		r.Bits = 16 // what the MP3 and AAC encoders take
	}
	if src.Bits > r.Bits && (f.ID == "wav16" || f.ID == "flac") {
		r.Notes = append(r.Notes, fmt.Sprintf("%d-bit becomes %d-bit", src.Bits, r.Bits))
	}

	// Bitrate: the chosen one when the encoder offers it at this rate and
	// channel count (mono MP3 stops lower), else the best it has below.
	if len(f.Bitrate) > 0 {
		r.Bitrate = o.Bitrate
		if r.Bitrate == 0 {
			r.Bitrate = f.Bitrate[0]
		}
		if offered := encoderBitrates(f, r.Rate, r.Channels); len(offered) > 0 && !contains(offered, r.Bitrate) {
			best := 0
			for _, k := range offered {
				if k < r.Bitrate && k > best {
					best = k
				}
			}
			if best == 0 {
				return Result{}, fmt.Errorf("the %s encoder has no %d kbps setting for %s %s",
					f.Name, r.Bitrate, khz(r.Rate), chName(r.Channels))
			}
			r.Notes = append(r.Notes, fmt.Sprintf("%s %s at %s goes up to %d kbps here: %d kbps becomes %d kbps",
				f.Name, chName(r.Channels), khz(r.Rate), best, r.Bitrate, best))
			r.Bitrate = best
		}
	}
	return r, nil
}

// closestRate picks the encoder rate nearest to hz, preferring its family
// (44.1 kHz multiples vs 48 kHz multiples).
func closestRate(rates []int, hz int) int {
	best, bestScore := rates[0], 1<<62
	for _, r := range rates {
		d := r - hz
		if d < 0 {
			d = -d
		}
		score := d
		if (r%44100 == 0) != (hz%44100 == 0) {
			score += 1 << 30 // other family: only if nothing else
		}
		if score < bestScore {
			best, bestScore = r, score
		}
	}
	return best
}

// Probe describes a sample: WAV and AIFF from their headers, other formats
// through the system's decoders.
func Probe(src string) (Info, error) {
	f, err := os.Open(src)
	if err != nil {
		return Info{}, err
	}
	var in audio.Info
	switch strings.ToLower(filepath.Ext(src)) {
	case ".wav":
		in, err = audio.WAVInfo(f)
	case ".aif", ".aiff":
		in, err = audio.AIFFInfo(f)
	default:
		f.Close()
		return probe(src)
	}
	f.Close()
	if err != nil {
		return probe(src) // e.g. WAV with a compressed codec
	}
	return Info{Rate: in.Rate, Channels: in.Channels, Bits: in.Bits, Seconds: in.Seconds}, nil
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
