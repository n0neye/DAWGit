//go:build !windows

package convert

// Other systems: macOS can use afconvert (no MP3 encoder there), Linux needs
// ffmpeg. Not done yet.
func convert(src, dst string, f Format, o Options) error { return ErrUnsupported }

func probe(src string) (Info, error) { return Info{}, ErrUnsupported }

func encoderBitrates(f Format, rate, channels int) []int { return nil }
