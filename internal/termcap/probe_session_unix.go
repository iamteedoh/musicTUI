//go:build unix

package termcap

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// ProbeSession holds the terminal in raw mode for a sequence of write-then-read
// exchanges — the transport for `musicTUI --artwork-probe`, which replays the
// artwork pipeline's graphics escapes with responses enabled and captures what
// the terminal says about them (MUS-34).
//
// Detect bundles its single exchange behind a fixed DA1 fence; a probe session
// instead lets the caller pick a completion predicate per exchange, because a
// graphics reply has no fixed shape (OK, an error, or nothing at all) and can
// arrive on the terminal's own schedule.
type ProbeSession struct {
	inFd uintptr
	old  *term.State
}

// OpenProbeSession puts the terminal in raw mode so replies arrive unbuffered
// and unechoed. It refuses when stdin/stdout is not a real terminal, or under
// a multiplexer (the escapes would need passthrough wrapping). Callers must
// Close the session to restore the terminal.
func OpenProbeSession() (*ProbeSession, error) {
	inFd := os.Stdin.Fd()
	if !term.IsTerminal(inFd) || !term.IsTerminal(os.Stdout.Fd()) {
		return nil, errors.New("stdin and stdout must be a terminal (not redirected)")
	}
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return nil, errors.New("running inside tmux/screen — graphics escapes would need passthrough wrapping")
	}
	old, err := term.MakeRaw(inFd)
	if err != nil {
		return nil, err
	}
	return &ProbeSession{inFd: inFd, old: old}, nil
}

// RoundTrip writes payload to the terminal, then reads the reply until done
// reports the bytes collected so far complete or the timeout elapses. An empty
// payload just reads — for grace-waiting on a reply that trails an exchange.
func (s *ProbeSession) RoundTrip(payload string, done func([]byte) bool, timeout time.Duration) ([]byte, error) {
	if payload != "" {
		if _, err := os.Stdout.WriteString(payload); err != nil {
			return nil, err
		}
	}
	return readUntil(int(s.inFd), timeout, done), nil
}

// Close restores the terminal. Safe to call more than once.
func (s *ProbeSession) Close() {
	if s.old != nil {
		_ = term.Restore(s.inFd, s.old)
		s.old = nil
	}
}
