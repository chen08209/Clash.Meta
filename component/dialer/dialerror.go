package dialer

import (
	"errors"
	"strings"
	"syscall"
)

// IsConnectionRefused reports whether err means nothing was listening at the
// destination. Windows reports the raw WSA code, which does not compare equal
// to syscall.ECONNREFUSED, so the message is checked as well.
func IsConnectionRefused(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "actively refused")
}
