package main

import "golang.org/x/sys/windows"

// shellOpen opens a file or folder like double-clicking it in Explorer,
// without a console window (unlike `cmd /c start`).
func shellOpen(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
