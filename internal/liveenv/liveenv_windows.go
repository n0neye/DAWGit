package liveenv

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// fileVersion is the file version in an executable's version resource
// ("12.3.2"), or "".
func fileVersion(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])) != nil {
		return ""
	}
	// The version as text (Live's fixed version numbers are placeholders),
	// in the first language the resource has.
	var tr *[2]uint16
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &n) != nil ||
		tr == nil || n < 4 {
		return ""
	}
	for _, key := range []string{"FileVersion", "ProductVersion"} {
		var text *uint16
		sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, tr[0], tr[1], key)
		if windows.VerQueryValue(unsafe.Pointer(&buf[0]), sub, unsafe.Pointer(&text), &n) == nil && text != nil && n > 0 {
			if v := strings.TrimSpace(windows.UTF16PtrToString(text)); v != "" {
				return v
			}
		}
	}
	return ""
}

// extraInstalls are the folders of Lives in the system's list of
// installed programs (Live may be installed elsewhere than ProgramData).
func extraInstalls() []string {
	var out []string
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, path := range []string{`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
			`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`} {
			k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
			if err != nil {
				continue
			}
			names, _ := k.ReadSubKeyNames(-1)
			k.Close()
			for _, n := range names {
				sk, err := registry.OpenKey(root, path+`\`+n, registry.QUERY_VALUE)
				if err != nil {
					continue
				}
				name, _, _ := sk.GetStringValue("DisplayName")
				loc, _, _ := sk.GetStringValue("InstallLocation")
				sk.Close()
				if strings.HasPrefix(name, "Ableton Live") && loc != "" {
					out = append(out, strings.TrimRight(loc, `\`))
				}
			}
		}
	}
	return out
}
