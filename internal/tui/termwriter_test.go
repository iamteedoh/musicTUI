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
// artwork kept vanishing (MUS-29).
func TestQueuedPayloadIsWrittenAfterTheFrame(t *testing.T) {
	tw, read := tempWriter(t)

	tw.Queue("<KITTY>")
	if got := read(); got != "" {
		t.Fatalf("queue wrote %q before any frame", got)
	}

	if _, err := tw.Write([]byte("<FRAME>")); err != nil {
		t.Fatal(err)
	}

	got := read()
	if got != "<FRAME><KITTY>" {
		t.Fatalf("got %q, want frame then payload", got)
	}

	// Staged payloads are one-shot: the next frame must not repaint a stale image.
	if _, err := tw.Write([]byte("<FRAME2>")); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "<FRAME><KITTY><FRAME2>" {
		t.Fatalf("payload was re-emitted: %q", got)
	}
}

// A cursor-positioned repaint must render as ONE update with the frame that
// blanks its cells, or the terminal draws the blank state on its own refresh
// and the cover blinks on every lyric (MUS-30).
func TestAtomicPayloadRendersWithTheFrame(t *testing.T) {
	tw, read := tempWriter(t)

	tw.QueueAtomic("<SIXEL>")
	if _, err := tw.Write([]byte("<FRAME>")); err != nil {
		t.Fatal(err)
	}

	if got := read(); got != beginSyncUpdate+"<FRAME><SIXEL>"+endSyncUpdate {
		t.Fatalf("got %q, want frame then payload inside one synchronized update", got)
	}

	// A bare frame needs no wrapper — 60 times a second, for nothing.
	if _, err := tw.Write([]byte("<FRAME2>")); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != beginSyncUpdate+"<FRAME><SIXEL>"+endSyncUpdate+"<FRAME2>" {
		t.Fatalf("a bare frame was wrapped, or the payload was re-emitted: %q", got)
	}
}

// Kitty payloads must go out EXACTLY as they did before synchronized output
// existed. A kitty image rides on placeholder cells the terminal redraws
// itself, so no frame can erase it and there is no gap to close — while the
// transmit is ~1 MB, and holding a terminal's render back for a megabyte to
// arrive is how covers stopped appearing in Ghostty at all.
func TestKittyTransmitIsNotSynchronized(t *testing.T) {
	tw, read := tempWriter(t)

	tw.Queue("\x1b_Ga=t,q=2,f=100,i=42,m=0;PAYLOAD\x1b\\")
	if _, err := tw.Write([]byte("<FRAME>")); err != nil {
		t.Fatal(err)
	}

	got := read()
	if strings.Contains(got, beginSyncUpdate) || strings.Contains(got, endSyncUpdate) {
		t.Fatalf("a kitty transmit was wrapped in a synchronized update: %q", got)
	}
	if got != "<FRAME>\x1b_Ga=t,q=2,f=100,i=42,m=0;PAYLOAD\x1b\\" {
		t.Fatalf("kitty bytes were not passed through unchanged: %q", got)
	}
}

// A lone repaint (no frame to hide) must NOT be wrapped: there is no erase to
// make atomic, and an unpaired BSU on a terminal that honors it stalls the
// screen until its timeout.
func TestStaleFlushIsNotSynchronized(t *testing.T) {
	tw, read := tempWriter(t)
	tw.Queue("<SIXEL>")
	tw.FlushStale(0)

	if got := read(); got != "<SIXEL>" {
		t.Fatalf("got %q, want a bare payload", got)
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
	// Every synchronized update must be closed, whichever goroutine raced:
	// an unpaired BSU leaves the terminal holding its screen back.
	if b, e := strings.Count(got, beginSyncUpdate), strings.Count(got, endSyncUpdate); b != e {
		t.Fatalf("%d synchronized updates opened but %d closed", b, e)
	}
}
