package livecheck

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processNames lists executable names via the Toolhelp API. Unlike running
// tasklist, it does not flash a console window when called from a GUI app.
func processNames() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var names []string
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		names = append(names, windows.UTF16ToString(e.ExeFile[:]))
	}
	return names, nil
}

var procGetWindowTextW = windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW")

// liveWindowTitles returns the titles of Ableton Live's visible windows.
func liveWindowTitles() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	pids := map[uint32]bool{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if isLive(windows.UTF16ToString(e.ExeFile[:])) {
			pids[e.ProcessID] = true
		}
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
		return 1 // continue
	})
	if len(pids) > 0 {
		windows.EnumWindows(cb, nil)
	}
	return titles, nil
}
