package als

import (
	"compress/gzip"
	"io"
	"os"
	"strings"
)

// CreatorOf returns the Live that last saved a set, e.g. "Ableton Live
// 12.3.1", reading only the start of the file (the root element's Creator
// attribute). "" when it cannot tell.
func CreatorOf(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	defer z.Close()
	buf := make([]byte, 2048)
	n, _ := io.ReadFull(z, buf)
	head := string(buf[:n])
	i := strings.Index(head, "<Ableton ")
	if i < 0 {
		return ""
	}
	head = head[i:]
	if end := strings.IndexByte(head, '>'); end > 0 {
		head = head[:end]
	}
	const key = `Creator="`
	j := strings.Index(head, key)
	if j < 0 {
		return ""
	}
	v := head[j+len(key):]
	if k := strings.IndexByte(v, '"'); k >= 0 {
		return v[:k]
	}
	return ""
}
