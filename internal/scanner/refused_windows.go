//go:build windows

package scanner

import (
	"errors"
	"syscall"
)

// dialRefused reports whether a dial error is a connection refusal. Windows
// reports a refused connect (RST) as WSAECONNREFUSED, errno 10061, which
// proves the host answered the probe.
func dialRefused(err error) bool {
	return errors.Is(err, syscall.Errno(10061))
}
