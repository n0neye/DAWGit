package teams

import (
	"errors"
	"strings"
)

// Secrets in teams.json (tokens, storage keys) are sealed where the system
// can (Windows: DPAPI, for this Windows user): another user of the
// computer, or the file copied elsewhere, can't read them. Older files with
// plain secrets are read as they are and sealed on the next save.

const sealedPrefix = "dpapi:"

// errSealedElsewhere: a secret sealed by another Windows user or computer.
var errSealedElsewhere = errors.New("sealed for another Windows user or computer")

func sealSecret(s string) string {
	if s == "" || strings.HasPrefix(s, sealedPrefix) {
		return s
	}
	if sealed, err := seal(s); err == nil {
		return sealed
	}
	return s // the system can't seal: kept as before
}

func unsealSecret(s string) (string, error) {
	if !strings.HasPrefix(s, sealedPrefix) {
		return s, nil
	}
	return unseal(s)
}
