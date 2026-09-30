//go:build !windows

package convert

// Other systems: macOS can use afconvert (no MP3 encoder there), Linux needs
// ffmpeg. Not done yet.
func convert(src, dst string, f Format, o Options) error { return ErrUnsupported }
