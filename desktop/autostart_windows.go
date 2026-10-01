package desktop

import (
	"dawgit/internal/version"
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// backgroundFlag starts DAWGit in the tray without opening the window
	// (used when Windows starts it at sign-in).
	backgroundFlag = "--background"
)

func autostartCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return `"` + exe + `" ` + backgroundFlag, nil
}

// Autostart reports whether DAWGit starts when the user signs in to Windows.
func (a *App) Autostart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue())
	return err == nil
}

// SetAutostart turns starting DAWGit at sign-in on or off.
func (a *App) SetAutostart(on bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		if err := k.DeleteValue(runValue()); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	cmd, err := autostartCommand()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValue(), cmd)
}

// runValue names the sign-in entry: the app's name, so a build with
// extensions ("DAWGit Pro") has its own next to the public app's.
func runValue() string { return version.Name() }
