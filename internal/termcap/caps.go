// Package termcap detects terminal capabilities by querying the terminal
// directly rather than guessing from environment variables.
package termcap

import (
	"bytes"
	"strconv"
	"strings"
)

// queryID is the arbitrary image id used in the kitty graphics support query;
// the terminal echoes it back in its reply.
const queryID = 4211

// Caps is what a single probe round-trip learned about the terminal.
//
// Every field is "false"/zero unless the terminal positively said otherwise,
// so a failed or skipped probe can only ever downgrade us to character art —
// never break a working setup.
type Caps struct {
	// Kitty reports support for the kitty graphics protocol (kitty, Ghostty).
	Kitty bool
	// Sixel reports support for sixel graphics, advertised as attribute 4 in
	// the Primary Device Attributes reply (Windows Terminal ≥ 1.22, WezTerm,
	// xterm -ti vt340, mintty, foot, Konsole).
	Sixel bool
	// CellW and CellH are the character cell size in pixels, needed to size a
	// sixel image to a whole number of cells. Zero when the terminal didn't
	// tell us — in which case sixel must not be used, because an image scaled
	// against a guessed cell size overflows or under-fills its panel.
	CellW, CellH int

	// Raw is the terminal's unparsed reply, for `musicTUI --caps`. Terminals
	// disagree wildly about these queries, so when artwork misbehaves the reply
	// is the only ground truth.
	Raw string
}

// RawEscaped renders Raw with control bytes shown as ^[ etc., safe to print.
func (c Caps) RawEscaped() string {
	var b strings.Builder
	for _, r := range c.Raw {
		switch {
		case r == 0x1b:
			b.WriteString("^[")
		case r < 0x20 || r == 0x7f:
			b.WriteString("^" + string(rune('@'+r)))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// query is the full probe payload: a kitty graphics support request, three
// XTWINOPS size reports, and a Primary Device Attributes request last.
//
// DA1 is the fence. Every terminal answers it, and it answers *after* the
// earlier queries, so seeing the DA1 reply means any kitty/size replies that
// were coming have already arrived. Terminals that don't implement a given
// query simply ignore it — none of these can hang.
//
//	\x1b_G…a=q…      kitty graphics support        → \x1b_G…;OK\x1b\
//	\x1b[16t         cell size in pixels           → \x1b[6;<h>;<w>t
//	\x1b[14t         text area size in pixels      → \x1b[4;<h>;<w>t
//	\x1b[18t         text area size in cells       → \x1b[8;<rows>;<cols>t
//	\x1b[c           primary device attributes     → \x1b[?<a>;<b>;…c
const query = "\x1b_Gi=" + "4211" + ",a=q,t=d,f=32,s=1,v=1;AAAAAA==\x1b\\" +
	"\x1b[16t\x1b[14t\x1b[18t\x1b[c"

// parseKittyReply reports whether buf contains kitty's positive graphics-support
// reply: an APC "G" response carrying ";OK". Terminals that don't implement the
// kitty graphics protocol ignore the query entirely and never emit an APC-G, so
// this is an unambiguous yes/no — unlike sniffing $TERM / $TERM_PROGRAM, which
// misidentifies Ghostty on Linux (MUS-20).
func parseKittyReply(buf []byte) bool {
	return bytes.Contains(buf, []byte("\x1b_G")) && bytes.Contains(buf, []byte(";OK"))
}

// parseSixelReply reports whether the DA1 reply advertises sixel graphics.
//
// DA1 looks like ESC [ ? 62 ; 4 ; 22 c — a semicolon-separated attribute list
// where the literal attribute "4" means sixel. Matching must be on whole
// parameters: "14" (or "4" inside "64") is not sixel support.
func parseSixelReply(buf []byte) bool {
	params, ok := da1Params(buf)
	if !ok {
		return false
	}
	for _, p := range params {
		if p == "4" {
			return true
		}
	}
	return false
}

// da1Params extracts the parameter list from a DA1 reply (ESC [ ? … c).
func da1Params(buf []byte) ([]string, bool) {
	s := string(buf)
	start := strings.Index(s, "\x1b[?")
	if start < 0 {
		return nil, false
	}
	end := strings.IndexByte(s[start:], 'c')
	if end < 0 {
		return nil, false
	}
	return strings.Split(s[start+3:start+end], ";"), true
}

// csiParams finds a CSI reply of the form ESC [ <lead> ; a ; b t and returns
// (a, b). Used for the XTWINOPS size reports, which all share that shape.
func csiParams(buf []byte, lead string) (int, int, bool) {
	s := string(buf)
	prefix := "\x1b[" + lead + ";"
	start := strings.Index(s, prefix)
	if start < 0 {
		return 0, 0, false
	}
	rest := s[start+len(prefix):]
	end := strings.IndexByte(rest, 't')
	if end < 0 {
		return 0, 0, false
	}
	parts := strings.Split(rest[:end], ";")
	if len(parts) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(parts[0])
	b, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
		return 0, 0, false
	}
	return a, b, true
}

// parseCellSize derives the pixel size of one character cell.
//
// Preferred source is XTWINOPS 16 (ESC[6;h;wt), which reports it directly.
// Terminals that don't implement 16 often implement 14 (text area in pixels)
// and 18 (text area in cells); dividing gives the same answer. Returns (0, 0)
// when neither is available, which disables sixel rather than guessing.
func parseCellSize(buf []byte) (w, h int) {
	if ch, cw, ok := csiParams(buf, "6"); ok {
		return cw, ch
	}
	pxH, pxW, okPx := csiParams(buf, "4")
	rows, cols, okCells := csiParams(buf, "8")
	if !okPx || !okCells {
		return 0, 0
	}
	// The division is only meaningful when the pixel size is exactly the cell
	// grid. Some terminals answer CSI 14 t with the *window* size, padding and
	// all, which inflates every cell by a pixel or two — enough for the image
	// to overflow the cells reserved for it and spill across the panel. An
	// inexact division is a tell that we're dividing the wrong rectangle, so
	// treat the cell size as unknown and fall back to character art.
	if pxW%cols != 0 || pxH%rows != 0 {
		return 0, 0
	}
	return pxW / cols, pxH / rows
}

// parseCaps turns a raw probe reply into Caps.
func parseCaps(buf []byte) Caps {
	c := Caps{
		Kitty: parseKittyReply(buf),
		Sixel: parseSixelReply(buf),
		Raw:   string(buf),
	}
	c.CellW, c.CellH = parseCellSize(buf)
	// A sixel image must be sized in whole pixels against a known cell; without
	// that we cannot place it correctly, so treat sixel as unsupported.
	if c.CellW <= 0 || c.CellH <= 0 {
		c.Sixel = false
	}
	return c
}

// indexDA1Terminator returns the index of the DA1 reply's terminating 'c'
// (after an ESC [ ? introducer), or -1 if not yet present.
func indexDA1Terminator(buf []byte) int {
	start := bytes.Index(buf, []byte("\x1b[?"))
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

// SupportsKittyGraphics reports kitty-graphics support only. Retained as the
// narrow question MUS-20 introduced; callers wanting the full picture (sixel,
// cell size) should use Detect.
func SupportsKittyGraphics() bool { return Detect().Kitty }
