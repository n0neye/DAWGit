package remote

import (
	"errors"
	"net"
	"net/http"
	"time"
)

// Transient: the request may work if made again (storage busy or briefly
// failing, the connection dropped), not refused for good.
func Transient(err error) bool {
	if err == nil {
		return false
	}
	var s3 *errS3
	if errors.As(err, &s3) {
		return s3.status >= 500 || s3.status == http.StatusTooManyRequests
	}
	var ne net.Error
	return errors.As(err, &ne)
}

// Retry runs fn until it works, fails for good, or failed transiently
// attempts times; it waits a little longer before each new try.
func Retry(attempts int, fn func() error) error {
	wait := RetryWait
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); err == nil || !Transient(err) {
			return err
		}
		if i < attempts-1 {
			time.Sleep(wait)
			wait *= 2
		}
	}
	return err
}

// RetryWait is the first wait before trying again (tests shorten it).
var RetryWait = time.Second

// RetryAttempts is how often a request to storage is tried.
const RetryAttempts = 4
