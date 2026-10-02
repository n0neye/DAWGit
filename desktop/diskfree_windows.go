package desktop

import "golang.org/x/sys/windows"

// diskFree is the space free for the user on the disk of dir (an existing
// folder), or -1 when unknown.
func diskFree(dir string) int64 {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return -1
	}
	var free, total, all uint64
	if windows.GetDiskFreeSpaceEx(p, &free, &total, &all) != nil {
		return -1
	}
	return int64(free)
}
