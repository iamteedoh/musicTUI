//go:build unix

package termcap

import (
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// Detect asks the terminal what it supports by writing the probe query and
// reading the replies before the DA1 fence arrives.
//
// This replaces environment-variable sniffing as the authoritative signal:
// Ghostty on Linux advertises inconsistent $TERM / $TERM_PROGRAM depending on
// installed terminfo and launch method, so env detection silently fell back to
// block art even though the terminal fully supports pixel graphics (MUS-20).
//
// Safety guarantees (so this can never regress a working setup):
//   - Only probes a real TTY — returns zero Caps when stdin/stdout are
//     redirected (pipes, CI, tests).
//   - Skips multiplexers (tmux/screen), which need APC passthrough.
//   - Never blocks past the timeout (poll(2)-bounded reads, no leaked goroutine).
//   - Always restores the terminal.
//   - Returns zero Caps on ANY error, so the caller falls back to env detection.
//     The probe can only upgrade a missed terminal to "supported", never the
//     reverse.
func Detect() Caps {
	inFd := os.Stdin.Fd() // uintptr, for x/term
	if !term.IsTerminal(inFd) || !term.IsTerminal(os.Stdout.Fd()) {
		return Caps{}
	}
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return Caps{}
	}

	old, err := term.MakeRaw(inFd)
	if err != nil {
		return Caps{}
	}
	defer func() { _ = term.Restore(inFd, old) }()

	if _, err := os.Stdout.WriteString(query); err != nil {
		return Caps{}
	}

	return parseCaps(readReply(int(inFd), 300*time.Millisecond))
}

// readReply drains fd until the DA1 reply's `c` terminator arrives or the total
// timeout elapses. DA1 is Detect's fence: its terminating 'c' means every
// earlier reply (kitty graphics OK, the size reports) is already buffered.
func readReply(fd int, total time.Duration) []byte {
	return readUntil(fd, total, func(buf []byte) bool {
		return indexDA1Terminator(buf) >= 0
	})
}

// readUntil drains fd until done reports the buffer complete or the total
// timeout elapses, using poll(2) so it never blocks past the deadline.
func readUntil(fd int, total time.Duration, done func([]byte) bool) []byte {
	deadline := time.Now().Add(total)
	var buf []byte
	tmp := make([]byte, 256)
	for !done(buf) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(pfd, int(remaining.Milliseconds()))
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			break
		}
		if n == 0 {
			break // timeout
		}
		if pfd[0].Revents&unix.POLLIN == 0 {
			continue
		}
		m, _ := unix.Read(fd, tmp)
		if m <= 0 {
			break
		}
		buf = append(buf, tmp[:m]...)
	}
	return buf
}
