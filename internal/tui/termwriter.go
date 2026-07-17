package tui

import (
	"os"
	"sync"
	"time"
)

// TermWriter serializes every byte that reaches the terminal, and sequences
// graphics payloads to land immediately after a frame rather than racing it.
//
// Bubble Tea flushes frames from its own goroutine under its own mutex. Kitty
// and sixel payloads are written out of band, because they carry image data
// that must not be diffed or truncated like view content. With two
// unsynchronized writers on one file descriptor, a sixel DCS payload could
// interleave with a frame (tearing), or land BEFORE the frame that blanks the
// cells it occupies — whereupon the frame promptly erased it. That is why the
// artwork flickered, showed torn bands, or vanished entirely (MUS-29).
//
// Queue stages a payload; Write flushes it directly after the frame bytes,
// while still holding the lock. So the pixels are always painted onto cells the
// terminal has already blanked, and nothing can interleave with them.
//
// It embeds *os.File because Bubble Tea type-asserts its output to term.File to
// read the terminal size — a plain io.Writer wrapper would silently disable
// resize handling.
type TermWriter struct {
	*os.File

	mu sync.Mutex
	// pending is the staged payload; atomic records that it must render
	// together with the frame rather than merely after it (see QueueAtomic).
	pending  []byte
	atomic   bool
	queuedAt time.Time
}

// Synchronized output — DEC private mode 2026, "BSU/ESU". Between these the
// terminal parses as usual but renders NOTHING, then applies the whole batch
// as one atomic update.
//
// This is what stops a CURSOR-POSITIONED cover (sixel, iTerm2) flickering.
// Painting one is inherently two steps: the frame blanks the cells (destroying
// the pixels on them), then the payload paints them back. A terminal refreshes
// on its own clock, so it is free to draw the screen in between — and it does:
// the artwork blinked out every time a lyric advanced, because the lyrics panel
// keeps the active line centered, so ALL of its lines change and Bubble Tea
// rewrites every line the cover sits on (MUS-30). Inside a synchronized update
// the blank state is never rendered, so there is nothing to see.
//
// It is deliberately NOT used for kitty payloads. Kitty placeholders are text
// cells the terminal redraws the image from itself, so no frame can blank them
// and there is no gap to hide — while an image transmit is ~1 MB, and holding a
// terminal's render back for a megabyte to arrive buys nothing and costs
// whatever that terminal does when a synchronized update overruns. Wrap only
// what needs wrapping; QueueAtomic is how a caller says so.
//
// Unsupported modes are ignored by definition, so terminals without it are no
// worse off than before. A terminal that does support it must also time the
// update out (the spec requires it), so a write that dies between BSU and ESU
// cannot wedge the screen.
const (
	beginSyncUpdate = "\x1b[?2026h"
	endSyncUpdate   = "\x1b[?2026l"
)

func NewTermWriter(f *os.File) *TermWriter { return &TermWriter{File: f} }

// Write emits p (a rendered frame), then any staged graphics payload, in one
// write.
//
// A payload staged by QueueAtomic additionally wraps the pair in a
// synchronized update: the frame's erase and the image's repaint have to reach
// the terminal as one indivisible unit, or the erase is visible on its own.
// Everything else — a bare frame, or a kitty transmit that no frame can erase —
// is passed through as the same bytes it always was.
func (t *TermWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 {
		return t.File.Write(p)
	}

	buf := make([]byte, 0, len(beginSyncUpdate)+len(p)+len(t.pending)+len(endSyncUpdate))
	if t.atomic {
		buf = append(buf, beginSyncUpdate...)
	}
	buf = append(buf, p...)
	buf = append(buf, t.pending...)
	if t.atomic {
		buf = append(buf, endSyncUpdate...)
	}
	t.pending, t.atomic = nil, false
	t.queuedAt = time.Time{}

	if _, err := t.File.Write(buf); err != nil {
		return 0, err
	}
	// Report only the frame bytes: the caller handed us p, not the wrapper.
	return len(p), nil
}

// Queue stages graphics escapes for the moment after the next frame — for
// payloads the frame cannot damage, such as a kitty transmit or placement.
func (t *TermWriter) Queue(seq string) { t.queue(seq, false) }

// QueueAtomic stages graphics escapes that must render TOGETHER with the next
// frame rather than merely after it: an image painted at the cursor lives on
// cells that frame is about to blank, so a terminal that refreshes between the
// two shows the cover blinking. See the synchronized-update constants above.
func (t *TermWriter) QueueAtomic(seq string) { t.queue(seq, true) }

func (t *TermWriter) queue(seq string, atomic bool) {
	if seq == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending = append(t.pending, seq...)
	// Mixed payloads are wrapped as a whole: the atomic one sets the terms,
	// and a kitty payload inside the wrapper would merely be redundant.
	t.atomic = t.atomic || atomic
	if t.queuedAt.IsZero() {
		t.queuedAt = time.Now()
	}
}

// FlushStale writes a payload that has been waiting longer than maxAge.
//
// Normally a payload rides out on the very next frame. But Bubble Tea skips the
// write entirely when a frame is byte-identical to the last one, so on a static
// screen a queued image could wait indefinitely. If nothing has been drawn for
// maxAge, the last frame on screen is by definition the one that blanked our
// cells — so it is safe, and necessary, to paint now.
//
// No synchronized update here: that frame was rendered long ago, so this is a
// lone repaint with no erase to hide behind it.
func (t *TermWriter) FlushStale(maxAge time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.pending) == 0 || time.Since(t.queuedAt) < maxAge {
		return
	}
	t.flushLocked()
}

func (t *TermWriter) flushLocked() {
	if len(t.pending) == 0 {
		return
	}
	_, _ = t.File.Write(t.pending)
	t.pending, t.atomic = nil, false
	t.queuedAt = time.Time{}
}
