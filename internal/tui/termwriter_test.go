package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/term"
)

// Bubble Tea type-asserts its output to term.File to read the terminal size.
// A wrapper that isn't one silently disables resize handling — the app would
// render at 0x0 and show nothing.
func TestTermWriterIsATermFile(t *testing.T) {
	var _ term.File = (*TermWriter)(nil)
}

func tempWriter(t *testing.T) (*TermWriter, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return NewTermWriter(f), func() string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
}

// The whole point: a queued image must land AFTER the frame that blanks its
// cells. Reversed, the frame erases the pixels — which is exactly how the
// artwork kept vanishing.
func TestQueuedPayloadIsWrittenAfterTheFrame(t *testing.T) {
	tw, read := tempWriter(t)

	tw.Queue("<SIXEL>")
	if got := read(); got != "" {
		t.Fatalf("queue wrote %q before any frame", got)
	}

	if _, err := tw.Write([]byte("<FRAME>")); err != nil {
		t.Fatal(err)
	}

	got := read()
	if got != "<FRAME><SIXEL>" {
		t.Fatalf("got %q, want frame then payload", got)
	}

	// Staged payloads are one-shot: the next frame must not repaint a stale image.
	if _, err := tw.Write([]byte("<FRAME2>")); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "<FRAME><SIXEL><FRAME2>" {
		t.Fatalf("payload was re-emitted: %q", got)
	}
}

// Write must report only the frame bytes it was given, or Bubble Tea sees a
// short/over-long write.
func TestWriteReportsOnlyFrameBytes(t *testing.T) {
	tw, _ := tempWriter(t)
	tw.Queue("<A-VERY-LONG-SIXEL-PAYLOAD>")

	frame := []byte("<FRAME>")
	n, err := tw.Write(frame)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(frame) {
		t.Fatalf("Write returned n=%d, want %d", n, len(frame))
	}
}

// Bubble Tea skips writing a frame identical to the last one, so on a static
// screen a queued image would otherwise wait forever.
func TestFlushStaleDrawsWhenNoFrameArrives(t *testing.T) {
	tw, read := tempWriter(t)
	tw.Queue("<SIXEL>")

	tw.FlushStale(time.Hour) // too young — must not draw
	if got := read(); got != "" {
		t.Fatalf("flushed a fresh payload: %q", got)
	}

	tw.FlushStale(0) // stale — draw it
	if got := read(); got != "<SIXEL>" {
		t.Fatalf("stale payload not flushed: %q", got)
	}

	tw.FlushStale(0) // nothing left
	if got := read(); got != "<SIXEL>" {
		t.Fatalf("flushed twice: %q", got)
	}
}

// The renderer flushes from its own goroutine while the event loop queues
// payloads. Under -race this pins the invariant that both go through the lock.
func TestConcurrentQueueAndWrite(t *testing.T) {
	tw, read := tempWriter(t)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			tw.Queue("<S>")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = tw.Write([]byte("<F>"))
		}
	}()
	wg.Wait()
	tw.FlushStale(0)

	got := read()
	if strings.Count(got, "<F>") != 200 {
		t.Fatalf("lost frames: %d of 200", strings.Count(got, "<F>"))
	}
	if strings.Count(got, "<S>") != 200 {
		t.Fatalf("lost payloads: %d of 200", strings.Count(got, "<S>"))
	}
	// No payload may ever appear before the first frame.
	if strings.HasPrefix(got, "<S>") {
		t.Fatal("a payload was written before any frame")
	}
}
