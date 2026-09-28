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
