package cli

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// userPath edits the user's PATH (HKCU\Environment, no administrator
// rights), keeping its kind (REG_EXPAND_SZ when it uses %VARIABLES%), and
// tells running programs it changed.
func userPath(add bool, dir string) (bool, error) {
	changed, err := editRegistryPath(`Environment`, add, dir)
	if changed {
		broadcastEnvironment()
	}
	return changed, err
}

// editRegistryPath edits the Path value of HKCU\key (tests use another key).
func editRegistryPath(key string, add bool, dir string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	value, kind, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		value, kind = "", registry.EXPAND_SZ
	} else if err != nil {
		return false, err
	}
	expand := func(s string) string {
		if x, err := registry.ExpandString(s); err == nil {
			return x
		}
		return s
	}
	next, changed := editPath(value, dir, add, expand)
	if !changed {
		return false, nil
	}
	if kind == registry.SZ {
		err = k.SetStringValue("Path", next)
	} else {
		err = k.SetExpandStringValue("Path", next)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// broadcastEnvironment sends WM_SETTINGCHANGE "Environment": Explorer (and
// what it starts next) reads the new PATH.
func broadcastEnvironment() {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		smtoAbortIfHung = 0x0002
	)
	env, _ := syscall.UTF16PtrFromString("Environment")
	var res uintptr
	syscall.NewLazyDLL("user32.dll").NewProc("SendMessageTimeoutW").Call(hwndBroadcast, wmSettingChange, 0,
		uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&res)))
}
