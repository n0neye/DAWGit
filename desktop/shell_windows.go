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

// shellSelect opens Explorer with the file selected.
func shellSelect(path string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	exe, err := windows.UTF16PtrFromString("explorer.exe")
	if err != nil {
		return err
	}
	args, err := windows.UTF16PtrFromString(`/select,"` + path + `"`)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, exe, args, nil, windows.SW_SHOWNORMAL)
}

// shellEdit opens a text file in its editor, or Notepad when no program is
// set for its type (e.g. .yaml on a fresh Windows).
func shellEdit(path string) error {
	if err := shellOpen(path); err == nil {
		return nil
	}
	verb, _ := windows.UTF16PtrFromString("open")
	exe, _ := windows.UTF16PtrFromString("notepad.exe")
	arg, err := windows.UTF16PtrFromString(`"` + path + `"`)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, exe, arg, nil, windows.SW_SHOWNORMAL)
}
