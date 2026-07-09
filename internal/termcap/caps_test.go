package termcap

import (
	"os"
	"strings"
	"testing"
)

func TestParseKittyReply(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"kitty OK before DA1", []byte("\x1b_Gi=4211;OK\x1b\\\x1b[?62;c"), true},
		{"kitty OK with extra keys", []byte("\x1b_Gi=4211,q=2;OK\x1b\\"), true},
		{"DA1 only (no graphics support)", []byte("\x1b[?62;22c"), false},
		{"empty (timed out)", []byte(nil), false},
		{"graphics error reply (not OK)", []byte("\x1b_Gi=4211;ENOSUP\x1b\\"), false},
	}
	for _, c := range cases {
		if got := parseKittyReply(c.buf); got != c.want {
			t.Errorf("%s: parseKittyReply(%q) = %v, want %v", c.name, c.buf, got, c.want)
		}
	}
}

// Sixel is DA1 attribute 4 — as a WHOLE parameter. Substring matching would
// see the "4" inside "14" or "64" and enable sixel on terminals that can't do
// it, which paints garbage escape bytes across the screen.
func TestParseSixelReply(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"xterm with sixel", []byte("\x1b[?62;4;22c"), true},
		{"sixel first", []byte("\x1b[?4;62c"), true},
		{"sixel last", []byte("\x1b[?62;22;4c"), true},
		{"windows terminal 1.22", []byte("\x1b[?61;4;6;7;14;21;22;23;24;28;32;42c"), true},
		{"no sixel", []byte("\x1b[?62;22c"), false},
		{"14 must not match", []byte("\x1b[?62;14;22c"), false},
		{"64 must not match", []byte("\x1b[?64;22c"), false},
		{"41 must not match", []byte("\x1b[?41c"), false},
		{"no DA1 at all", []byte("\x1b_Gi=4211;OK\x1b\\"), false},
		{"empty", []byte(nil), false},
	}
	for _, c := range cases {
		if got := parseSixelReply(c.buf); got != c.want {
			t.Errorf("%s: parseSixelReply(%q) = %v, want %v", c.name, c.buf, got, c.want)
		}
	}
}

func TestParseCellSize(t *testing.T) {
	cases := []struct {
		name   string
		buf    []byte
		w, h   int
		reason string
	}{
		{
			name: "XTWINOPS 16 reports the cell directly",
			buf:  []byte("\x1b[6;20;10t\x1b[?62;4c"),
			w:    10, h: 20,
		},
		{
			name: "falls back to 14t/18t division",
			buf:  []byte("\x1b[4;800;400t\x1b[8;40;40t\x1b[?62;4c"),
			w:    10, h: 20, // 400/40 = 10, 800/40 = 20
		},
		{
			name: "16t wins over 14t/18t when both present",
			buf:  []byte("\x1b[6;18;9t\x1b[4;800;400t\x1b[8;40;40t\x1b[?62;4c"),
			w:    9, h: 18,
		},
		{
			name: "only 14t (no cell count) is unusable",
			buf:  []byte("\x1b[4;800;400t\x1b[?62;4c"),
			w:    0, h: 0,
		},
		{
			// Terminals that answer 14t with the WINDOW size (padding included)
			// make the division inexact. Trusting it inflates every cell and the
			// cover spills out of its panel — seen in Konsole (MUS-29).
			name: "inexact division is rejected, not rounded",
			buf:  []byte("\x1b[4;806;404t\x1b[8;40;40t\x1b[?62;4c"),
			w:    0, h: 0,
		},
		{
			name: "16t still wins even when 14t/18t would be inexact",
			buf:  []byte("\x1b[6;16;8t\x1b[4;806;404t\x1b[8;40;40t\x1b[?62;4c"),
			w:    8, h: 16,
		},
		{name: "no size replies at all", buf: []byte("\x1b[?62;4c"), w: 0, h: 0},
		{name: "zero dimensions rejected", buf: []byte("\x1b[6;0;0t"), w: 0, h: 0},
		{name: "garbage params rejected", buf: []byte("\x1b[6;x;yt"), w: 0, h: 0},
	}
	for _, c := range cases {
		w, h := parseCellSize(c.buf)
		if w != c.w || h != c.h {
			t.Errorf("%s: parseCellSize(%q) = (%d,%d), want (%d,%d)", c.name, c.buf, w, h, c.w, c.h)
		}
	}
}

// A sixel image has to be scaled against a known cell size to land on cell
// boundaries. If the terminal advertises sixel but won't tell us its cell size,
// we must NOT use sixel — a guessed size overflows the panel or under-fills it.
func TestParseCapsDisablesSixelWithoutCellSize(t *testing.T) {
	withSize := parseCaps([]byte("\x1b[6;20;10t\x1b[?62;4;22c"))
	if !withSize.Sixel || withSize.CellW != 10 || withSize.CellH != 20 {
		t.Fatalf("sixel + cell size: got %+v", withSize)
	}

	noSize := parseCaps([]byte("\x1b[?62;4;22c"))
	if noSize.Sixel {
		t.Fatalf("sixel enabled without a cell size: %+v", noSize)
	}
}

func TestParseCapsKittyAndSixelIndependent(t *testing.T) {
	// Ghostty: kitty graphics, no sixel.
	k := parseCaps([]byte("\x1b_Gi=4211;OK\x1b\\\x1b[6;20;10t\x1b[?62;22c"))
	if !k.Kitty || k.Sixel {
		t.Fatalf("kitty-only terminal: got %+v", k)
	}
	// Windows Terminal: sixel, no kitty graphics.
	s := parseCaps([]byte("\x1b[6;20;10t\x1b[?61;4;6;22c"))
	if s.Kitty || !s.Sixel {
		t.Fatalf("sixel-only terminal: got %+v", s)
	}
	// A terminal that answers nothing (probe timed out) must claim nothing.
	none := parseCaps(nil)
	if none.Kitty || none.Sixel || none.CellW != 0 || none.CellH != 0 {
		t.Fatalf("silent terminal: got %+v", none)
	}
}

// The probe must be side-effect-free and return false when stdout isn't a
// terminal — which is the case under `go test` (output is captured). This
// guards the guarantee that tests and piped runs never touch the terminal.
func TestDetectInertWhenNotTTY(t *testing.T) {
	if term := os.Getenv("TERM"); term == "" {
		// belt-and-suspenders: even with no TERM it must not panic
	}
	if got := Detect(); got.Kitty || got.Sixel {
		t.Fatalf("Detect reported capabilities under a non-TTY test harness: %+v", got)
	}
	if SupportsKittyGraphics() {
		t.Fatal("SupportsKittyGraphics returned true under a non-TTY test harness")
	}
}

// --caps prints the terminal's reply; control bytes must be escaped or the
// escape sequences would re-execute against the terminal reading the output.
func TestRawEscaped(t *testing.T) {
	c := parseCaps([]byte("\x1b[6;20;10t\x1b[?62;4c"))
	got := c.RawEscaped()
	want := "^[[6;20;10t^[[?62;4c"
	if got != want {
		t.Fatalf("RawEscaped() = %q, want %q", got, want)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Fatal("RawEscaped() left a raw ESC in the output")
	}
}
