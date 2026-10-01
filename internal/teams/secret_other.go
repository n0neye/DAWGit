//go:build !windows

package teams

import "errors"

// Elsewhere secrets are kept as they are (the file is readable by this user
// only); sealed ones came from Windows.
func seal(string) (string, error) { return "", errors.ErrUnsupported }

func unseal(string) (string, error) { return "", errSealedElsewhere }
