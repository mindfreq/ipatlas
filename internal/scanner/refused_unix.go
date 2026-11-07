//go:build !windows

package scanner

import (
	"errors"
	"syscall"
)

// dialRefused reports whether a dial error is a connection refusal. A refused
// connect (RST) proves the host answered the probe.
func dialRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED)
}
