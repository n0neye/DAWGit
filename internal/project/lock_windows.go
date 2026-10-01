package project

import (
	"errors"
	"syscall"
)

const errorSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION

// lockFile opens path shared with no one: another program can't open it
// until it is closed, which Windows does when this program ends.
func lockFile(path string) (func(), error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil,
		syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errorSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return nil, errLocked
		}
		return nil, err
	}
	return func() { syscall.CloseHandle(h) }, nil
}
