package components

import (
	"image"
	"image/color"
	"strings"
	"time"
)

// Artwork probe (MUS-34): replay the kitty-graphics artwork pipeline outside
// Bubble Tea with terminal responses ENABLED, and report the verdict.
//
// The app has to transmit covers with q=2 — suppressing the OK and any ERROR
// response alike, because a reply would land in Bubble Tea's input stream and
// be parsed as keystrokes. That safety means a terminal rejecting a specific
// cover rejects it silently: the panel just stays empty, and the terminal's
// own explanation (EFBIG, ENODATA, a quota, …) is thrown away. This probe is
// the counterpart that gets the explanation back out: the same PNG encode,
// the same chunking, the same escapes and image id — but verbose, on a quiet
// raw tty where the reply can be read directly (the same trick
// internal/termcap's Detect uses), before any event loop exists.

const (
	// probePanelCols/Rows: a nominal artwork panel for the probe's virtual
	// placement — acceptance doesn't depend on the exact size, it only has to
	// exercise the same grid math the app runs. 24×12 fits any sane terminal
	// and maps a square cover to a 24×12 placeholder grid.
	probePanelCols = 24
	probePanelRows = 12

	// probeReplyTimeout bounds the wait for the terminal's two replies. A
	// terminal that implements the protocol answers in milliseconds once it
	// has consumed the payload; one that doesn't will never answer, and the
	// probe reports that instead.
	probeReplyTimeout = 4 * time.Second
)

// ProbeTTY is the raw-terminal transport the probe runs over — implemented by
// termcap.ProbeSession. done receives every byte read so far in this exchange.
type ProbeTTY interface {
	RoundTrip(payload string, done func([]byte) bool, timeout time.Duration) ([]byte, error)
}

// KittyProbeReport is everything one probe run learned.
type KittyProbeReport struct {
	ID         uint32
	SrcW, SrcH int
	Cols, Rows int
	PNGBytes   int
	B64Chars   int
	Chunks     int

	// The APC-G reply payloads, e.g. "i=123;OK" or "i=123;EFBIG:…".
	// Empty when no reply arrived — a terminal without kitty graphics
	// ignores the commands entirely.
	TransmitReply  string
	PlacementReply string

	// Raw is every byte the terminal sent back, unparsed ground truth.
	Raw string

	// EncodeErr set means png.Encode failed and nothing was sent — in the app
	// this cover falls back to block art (the "chunky blocks" symptom).
	EncodeErr error
	// WriteErr set means the escapes couldn't be written to the terminal.
	WriteErr error
}

// TransmitOK reports whether the terminal explicitly accepted the transmit.
func (r KittyProbeReport) TransmitOK() bool { return kittyReplyOK(r.TransmitReply) }

// PlacementOK reports whether the terminal explicitly accepted the placement.
func (r KittyProbeReport) PlacementOK() bool { return kittyReplyOK(r.PlacementReply) }

// PlaceholderGrid renders the Unicode-placeholder grid for the probed image,
// exactly as the app would print it. On a working pipeline the terminal draws
// the image over these cells — making "transmit accepted but placeholders
// don't render" (a text/color-layer fault) visibly distinct from "transmit
// rejected" (a payload fault).
func (r KittyProbeReport) PlaceholderGrid() string {
	var rows []string
	for row := 0; row < r.Rows; row++ {
		rows = append(rows, kittyPlaceholderRow(r.ID, row, r.Cols))
	}
	return strings.Join(rows, "\n")
}

func kittyReplyOK(reply string) bool { return strings.HasSuffix(reply, ";OK") }

// kittyGraphicsReplies extracts the payload of every complete APC-G response
// in buf ("\x1b_G<payload>\x1b\\" → "<payload>"), in order. An incomplete
// trailing response is left for a later read.
func kittyGraphicsReplies(buf []byte) []string {
	var out []string
	s := string(buf)
	for {
		start := strings.Index(s, "\x1b_G")
		if start < 0 {
			return out
		}
		s = s[start+3:]
		end := strings.Index(s, "\x1b\\")
		if end < 0 {
			return out
		}
		out = append(out, s[:end])
		s = s[end+2:]
	}
}

// ProbeKittyArtwork transmits img over tty exactly as the app's kitty tier
// would — same id derivation from src, same PNG encode, chunking and virtual
// placement — but verbose, and returns what the terminal said about it.
// It cleans up after itself by deleting the transmitted image.
func ProbeKittyArtwork(src string, img image.Image, tty ProbeTTY) KittyProbeReport {
	bounds := img.Bounds()
	r := KittyProbeReport{SrcW: bounds.Dx(), SrcH: bounds.Dy()}
	r.ID = kittyImageID(src)
	r.Cols, r.Rows = kittyGridSize(r.SrcW, r.SrcH, probePanelCols, probePanelRows)

	tx, stats, err := kittyTransmit(r.ID, img, 0)
	if err != nil {
		r.EncodeErr = err
		return r
	}
	r.PNGBytes, r.B64Chars, r.Chunks = stats.PNGBytes, stats.B64Chars, stats.Chunks
	artworkDebugf("probe: transmit id=%d src=%dx%d grid=%dx%d png=%dB b64=%d chunks=%d src=%s",
		r.ID, r.SrcW, r.SrcH, r.Cols, r.Rows, r.PNGBytes, r.B64Chars, r.Chunks, src)

	// Transmit and placement go out together; the terminal processes commands
	// in order, so the first reply answers the transmit and the second the
	// placement. Verbose commands are guaranteed a reply from a terminal that
	// implements the protocol, so silence past the timeout is itself the
	// diagnosis ("this terminal ignores kitty graphics").
	payload := tx + kittyPlacement(r.ID, r.Cols, r.Rows, 0)
	raw, err := tty.RoundTrip(payload, func(b []byte) bool {
		return len(kittyGraphicsReplies(b)) >= 2
	}, probeReplyTimeout)
	if err != nil {
		r.WriteErr = err
		return r
	}
	r.Raw = string(raw)

	replies := kittyGraphicsReplies(raw)
	if len(replies) > 0 {
		r.TransmitReply = replies[0]
	}
	if len(replies) > 1 {
		r.PlacementReply = replies[1]
	}
	artworkDebugf("probe: transmit reply=%q placement reply=%q", r.TransmitReply, r.PlacementReply)

	// Deliberately NO delete here. Freeing the image (a=d,d=I) would take its
	// virtual placement with it, and the placeholder grid the caller prints
	// next would reference an image the terminal no longer has — rendering as
	// literal placeholder glyphs and framing a working terminal as broken.
	// The image stays in the terminal's storage until the window closes; a
	// re-probe of the same source reuses the same id and replaces it.
	return r
}

// ProbeTestImage is the cover the probe uses when no file or URL is given: a
// deterministic 640×640 gradient with per-pixel noise in the low bits, so its
// PNG lands in the same hundreds-of-kilobytes class as a real album cover
// rather than compressing away to nothing.
func ProbeTestImage() image.Image {
	const size = 640
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	rnd := uint32(1)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			// xorshift32: deterministic noise, no seeding concerns.
			rnd ^= rnd << 13
			rnd ^= rnd >> 17
			rnd ^= rnd << 5
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(x*255/(size-1)) ^ uint8(rnd&0x1f),
				G: uint8(y*255/(size-1)) ^ uint8((rnd>>5)&0x1f),
				B: uint8((x+y)*255/(2*size-2)) ^ uint8((rnd>>10)&0x1f),
				A: 255,
			})
		}
	}
	return img
}
