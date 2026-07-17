//go:build !unix

package termcap

import (
	"errors"
	"time"
)

// ProbeSession is unavailable off POSIX: the artwork probe targets the kitty
// graphics tier, whose terminals (kitty, Ghostty) are POSIX-only. The Windows
// console transport in probe_windows.go could carry it if a Windows terminal
// ever grows Unicode-placeholder support.
type ProbeSession struct{}

// OpenProbeSession always fails on this platform.
func OpenProbeSession() (*ProbeSession, error) {
	return nil, errors.New("the artwork probe requires a POSIX terminal; it is not implemented for this platform")
}

// RoundTrip always fails on this platform.
func (s *ProbeSession) RoundTrip(string, func([]byte) bool, time.Duration) ([]byte, error) {
	return nil, errors.New("the artwork probe is not implemented for this platform")
}

// Close is a no-op on this platform.
func (s *ProbeSession) Close() {}
