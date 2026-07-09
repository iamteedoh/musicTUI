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

	mu       sync.Mutex
	pending  []byte
	queuedAt time.Time
}

func NewTermWriter(f *os.File) *TermWriter { return &TermWriter{File: f} }

// Write emits p (a rendered frame), then any staged graphics payload.
func (t *TermWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n, err := t.File.Write(p)
	if err != nil {
		return n, err
	}
	t.flushLocked()
	return n, nil
}

// Queue stages graphics escapes for the moment after the next frame.
func (t *TermWriter) Queue(seq string) {
	if seq == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending = append(t.pending, seq...)
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
	t.pending = nil
	t.queuedAt = time.Time{}
}
