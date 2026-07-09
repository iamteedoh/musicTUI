//go:build windows

package termcap

import (
	"os"
	"time"

	"github.com/erikgeiser/coninput"
	"golang.org/x/sys/windows"
)

// Detect asks the Windows console what it supports.
//
// Windows had no probe at all before this: detection fell straight through to
// environment heuristics, so a terminal that can render pixels (Windows
// Terminal ≥ 1.22 speaks sixel) only ever got block art (MUS-29).
//
// Same safety guarantees as the POSIX probe, plus one that matters more here:
// it is built on PeekConsoleInput, which never blocks. A blocking read on a
// console handle cannot be interrupted by a timeout, so a terminal that simply
// ignored our query would hang musicTUI at startup forever.
//
//   - Only probes a real console — zero Caps when stdin/stdout are redirected.
//   - Never blocks: polls with Peek, sleeping between empty polls.
//   - Always restores both console modes.
//   - Returns zero Caps on ANY error, so we can only ever upgrade a terminal
//     to "supported", never break a working one.
func Detect() Caps {
	in, err := coninput.NewStdinHandle()
	if err != nil {
		return Caps{}
	}
	out := windows.Handle(os.Stdout.Fd())

	var inMode, outMode uint32
	if err := windows.GetConsoleMode(in, &inMode); err != nil {
		return Caps{} // stdin is redirected — not a console
	}
	if err := windows.GetConsoleMode(out, &outMode); err != nil {
		return Caps{} // stdout is redirected
	}
	defer func() {
		_ = windows.SetConsoleMode(in, inMode)
		_ = windows.SetConsoleMode(out, outMode)
	}()

	// Raw-ish input: no line buffering, no echo, no Ctrl-C translation, and —
	// critically — no mouse or window events, so the only records that can
	// appear are the key events carrying the terminal's reply.
	rawIn := coninput.RemoveInputModes(inMode,
		windows.ENABLE_LINE_INPUT,
		windows.ENABLE_ECHO_INPUT,
		windows.ENABLE_PROCESSED_INPUT,
		windows.ENABLE_MOUSE_INPUT,
		windows.ENABLE_WINDOW_INPUT,
	)
	rawIn = coninput.AddInputModes(rawIn, windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	if err := windows.SetConsoleMode(in, rawIn); err != nil {
		return Caps{}
	}
	// The reply is ANSI, so the console must be interpreting VT on output too.
	if err := windows.SetConsoleMode(out,
		outMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return Caps{}
	}

	if err := coninput.FlushConsoleInputBuffer(in); err != nil {
		return Caps{}
	}
	if _, err := os.Stdout.WriteString(query); err != nil {
		return Caps{}
	}

	return parseCaps(readConsoleReply(in, 300*time.Millisecond))
}

// readConsoleReply collects the characters the terminal wrote back, stopping at
// the DA1 terminator or the deadline — whichever comes first. It only ever
// reads records it has already seen via Peek, so it cannot block.
func readConsoleReply(in windows.Handle, total time.Duration) []byte {
	deadline := time.Now().Add(total)
	var buf []byte
	for time.Now().Before(deadline) {
		n, err := coninput.GetNumberOfConsoleInputEvents(in)
		if err != nil {
			break
		}
		if n == 0 {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		records, err := coninput.ReadNConsoleInputs(in, n)
		if err != nil {
			break
		}
		for _, rec := range records {
			key, ok := rec.Unwrap().(coninput.KeyEventRecord)
			if !ok || !key.KeyDown || key.Char == 0 {
				continue
			}
			buf = append(buf, []byte(string(key.Char))...)
		}
		if i := indexDA1Terminator(buf); i >= 0 {
			break
		}
	}
	return buf
}
