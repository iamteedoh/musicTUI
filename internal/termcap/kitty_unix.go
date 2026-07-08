//go:build unix

package termcap

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

// SupportsKittyGraphics asks the terminal whether it implements the kitty
// graphics protocol by sending a support query (a=q) followed by a Primary
// Device Attributes request as a fence, then watching for the kitty "OK" reply
// before the DA1 reply arrives.
//
// This replaces environment-variable sniffing as the authoritative signal:
// Ghostty on Linux advertises inconsistent $TERM / $TERM_PROGRAM depending on
// installed terminfo and launch method, so env detection silently fell back to
// block art even though the terminal fully supports pixel graphics (MUS-20).
//
// Safety guarantees (so this can never regress a working setup):
//   - Only probes a real TTY — returns false when stdin/stdout are redirected
//     (pipes, CI, tests).
//   - Skips multiplexers (tmux/screen), which need APC passthrough.
//   - Never blocks past the timeout (poll(2)-bounded reads, no leaked goroutine).
//   - Always restores the terminal.
//   - Returns false on ANY error, so the caller falls back to env detection.
//     The probe can only upgrade a missed terminal to "supported", never the
//     reverse.
func SupportsKittyGraphics() bool {
	inFd := os.Stdin.Fd() // uintptr, for x/term
	if !term.IsTerminal(inFd) || !term.IsTerminal(os.Stdout.Fd()) {
		return false
	}
	if os.Getenv("TMUX") != "" || strings.HasPrefix(os.Getenv("TERM"), "screen") {
		return false
	}

	old, err := term.MakeRaw(inFd)
	if err != nil {
		return false
	}
	defer func() { _ = term.Restore(inFd, old) }()

	// a=q graphics query with a 1x1 32-bit direct-transmit image, then DA1.
	query := "\x1b_Gi=" + strconv.Itoa(queryID) + ",a=q,t=d,f=32,s=1,v=1;AAAAAA==\x1b\\\x1b[c"
	if _, err := os.Stdout.WriteString(query); err != nil {
		return false
	}

	return parseKittyReply(readReply(int(inFd), 300*time.Millisecond))
}

// readReply drains fd until the DA1 reply's `c` terminator arrives or the total
// timeout elapses, using poll(2) so it never blocks past the deadline.
func readReply(fd int, total time.Duration) []byte {
	deadline := time.Now().Add(total)
	var buf []byte
	tmp := make([]byte, 256)
	for {
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
		// DA1 reply is ESC [ ? … c. Its terminating 'c' means every earlier
		// reply (the kitty graphics OK, which the terminal sends first) is
		// already buffered, so we can stop.
		if i := indexDA1Terminator(buf); i >= 0 {
			break
		}
	}
	return buf
}

// indexDA1Terminator returns the index of the DA1 reply's terminating 'c'
// (after an ESC [ ? introducer), or -1 if not yet present.
func indexDA1Terminator(buf []byte) int {
	start := strings.Index(string(buf), "\x1b[?")
	if start < 0 {
		return -1
	}
	for i := start + 3; i < len(buf); i++ {
		if buf[i] == 'c' {
			return i
		}
	}
	return -1
}
