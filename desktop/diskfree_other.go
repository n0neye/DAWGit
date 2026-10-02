//go:build !windows

package desktop

import "golang.org/x/sys/unix"

// diskFree is the space free for the user on the disk of dir (an existing
// folder), or -1 when unknown.
func diskFree(dir string) int64 {
	var st unix.Statfs_t
	if unix.Statfs(dir, &st) != nil {
		return -1
	}
	return int64(st.Bavail) * int64(st.Bsize)
}
