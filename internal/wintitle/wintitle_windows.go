// Package wintitle lists the titles of a program's visible windows, to tell
// which project an editor has open.
package wintitle

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetWindowTextW = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")

// Of returns the titles of the visible windows of programs named exe (e.g.
// "Unity.exe").
func Of(exe string) []string {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	pids := map[uint32]bool{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), exe) {
			pids[e.ProcessID] = true
		}
	}
	if len(pids) == 0 {
		return nil
	}
	var titles []string
	cb := windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var pid uint32
		windows.GetWindowThreadProcessId(hwnd, &pid)
		if pids[pid] && windows.IsWindowVisible(hwnd) {
			buf := make([]uint16, 512)
			n, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if n > 0 {
				titles = append(titles, windows.UTF16ToString(buf[:n]))
			}
		}
		return 1
	})
	windows.EnumWindows(cb, nil)
	return titles
}
