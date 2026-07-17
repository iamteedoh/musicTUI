package components

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"strings"
)

// Kitty graphics protocol — Unicode placeholder rendering.
//
// The terminal stores the image (transmitted once as PNG over an APC escape),
// and a "virtual placement" (U=1) binds it to a rectangle of rows×cols cells.
// The TUI then simply prints placeholder characters (U+10EEEE) whose
// foreground color encodes the image ID and whose combining diacritics encode
// the cell's row/column. Because the image rides on ordinary text cells, the
// Bubble Tea renderer can repaint, scroll, and diff freely — no cursor games.
//
// Supported by kitty and Ghostty (the only terminals implementing the
// Unicode-placeholder part of the spec); everything else falls back to the
// quadrant-block renderer. See https://sw.kovidgoyal.net/kitty/graphics-protocol/

// placeholderRune is U+10EEEE, the dedicated image-placeholder codepoint.
const placeholderRune = '\U0010EEEE'

// rowColDiacritics is the ordered diacritic table from kitty's
// rowcolumn-diacritics.txt: the Nth entry marks row/column N. The grid is
// capped at len(rowColDiacritics) cells per axis (40 — far above the artwork
// panel's size).
var rowColDiacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357,
	0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484,
	0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
}

// maxKittyGridDim is the largest placeholder grid we can address per axis.
func maxKittyGridDim() int { return len(rowColDiacritics) }

// kittyGridSize fits a srcW×srcH image into a charW×charH box of character
// cells using the 1-wide × 2-tall cell aspect every character renderer here
// assumes, capped by the diacritic table so each cell stays addressable.
// Shared by renderPlaceholders and the artwork probe, so the probe asks the
// terminal for exactly the grid the app would.
func kittyGridSize(srcW, srcH, charW, charH int) (cols, rows int) {
	scale := float64(srcW) / float64(charW)
	if s := float64(srcH) / float64(charH*2); s > scale {
		scale = s
	}
	cols = int(float64(srcW) / scale)
	rows = int(float64(srcH) / scale / 2)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if max := maxKittyGridDim(); cols > max {
		cols = max
	}
	if max := maxKittyGridDim(); rows > max {
		rows = max
	}
	return cols, rows
}

// ArtworkStyle selects how the artwork panel renders the cover.
type ArtworkStyle int

const (
	// StyleBlocks: quadrant block elements chosen by error minimization.
	// The universal default — photographs read smoothest.
	StyleBlocks ArtworkStyle = iota
	// StyleBraille: colored braille over painted backgrounds — higher
	// spatial detail, visible dot texture. Opt-in for those who prefer it.
	StyleBraille
	// StyleKitty: the actual image via kitty-graphics Unicode placeholders.
	StyleKitty
	// StyleSixel: the actual image via a sixel DCS payload. Real pixels for
	// terminals that don't implement kitty graphics — Windows Terminal,
	// WezTerm, xterm, mintty, foot, Konsole (MUS-29).
	StyleSixel
	// StyleITerm2: the actual image via iTerm2's native OSC 1337 File=
	// protocol, sized in cells. iTerm2 answers the kitty probe but renders
	// no placeholders, and reports no usable cell size for sixel (MUS-30).
	StyleITerm2
)

// String names the style the way MUSICTUI_ARTWORK spells it.
func (s ArtworkStyle) String() string {
	switch s {
	case StyleBraille:
		return "braille"
	case StyleKitty:
		return "kitty"
	case StyleSixel:
		return "sixel"
	case StyleITerm2:
		return "iterm2"
	}
	return "blocks"
}

// DetectArtworkStyle picks the artwork renderer. MUSICTUI_ARTWORK overrides
// detection: "kitty", "sixel" and "iterm2" force the respective pixel
// protocols, "blocks" and "braille" force the character-art styles.
//
// kittyProbe is the result of querying the terminal directly for kitty
// graphics support (see internal/termcap). It is the authoritative signal —
// env-var sniffing misidentifies Ghostty on Linux, which made pixel artwork
// silently fall back to block art (MUS-20). The env heuristic below is kept
// only as a fallback for when the probe couldn't run (e.g. not a TTY).
func DetectArtworkStyle(kittyProbe, sixelProbe bool) ArtworkStyle {
	style, reason := detectArtworkStyle(kittyProbe, sixelProbe)
	artworkDebugf("style: %s — %s (probe kitty=%t sixel=%t; lipgloss profile=%s)",
		style, reason, kittyProbe, sixelProbe, ColorProfileName())
	return style
}

// detectArtworkStyle is DetectArtworkStyle's decision tree, with the reason
// each branch was taken — that reason is exactly what an artwork bug report
// needs first, so the debug log records it.
func detectArtworkStyle(kittyProbe, sixelProbe bool) (ArtworkStyle, string) {
	switch strings.ToLower(os.Getenv("MUSICTUI_ARTWORK")) {
	case "blocks":
		return StyleBlocks, "MUSICTUI_ARTWORK override"
	case "braille":
		return StyleBraille, "MUSICTUI_ARTWORK override"
	case "kitty", "hires":
		return StyleKitty, "MUSICTUI_ARTWORK override"
	case "sixel":
		return StyleSixel, "MUSICTUI_ARTWORK override"
	case "iterm2", "iterm":
		return StyleITerm2, "MUSICTUI_ARTWORK override"
	}
	// Inside tmux the APC/DCS escapes would need passthrough wrapping; use blocks.
	if os.Getenv("TMUX") != "" {
		return StyleBlocks, "tmux (no APC/DCS passthrough)"
	}
	// iTerm2 must be identified before the kitty probe is consulted: it
	// answers a=q with ";OK" (≥ 3.6) but renders no Unicode placeholders, so
	// trusting the probe leaves a blank panel (MUS-30). Its native inline
	// protocol is sized in cells, so it needs no probe result at all. Locally
	// iTerm2 always sets TERM_PROGRAM; over ssh it forwards LC_TERMINAL when
	// the server accepts LC_* (a stripped environment falls through to the
	// probe and, like Konsole over ssh, cannot be told apart from kitty).
	if isITerm2() {
		return StyleITerm2, "iTerm2 named itself"
	}
	// Authoritative: the terminal told us it supports kitty graphics — but only
	// our renderer's dialect of it counts (see kittyPlaceholders).
	if kittyProbe && kittyPlaceholders() {
		return StyleKitty, "terminal answered the kitty graphics query"
	}
	// Next best real-pixel path. Preferred over kitty's env heuristic below
	// because it, too, came from the terminal itself rather than a guess.
	if sixelProbe {
		return StyleSixel, "terminal advertised sixel (DA1 attribute 4)"
	}
	// Fallback heuristic when the probe couldn't run (redirected output, etc.).
	term := os.Getenv("TERM")
	prog := os.Getenv("TERM_PROGRAM")
	if !kittyPlaceholders() {
		return StyleBlocks, "no probe answer; terminal denylisted for placeholders"
	}
	if strings.Contains(term, "kitty") || os.Getenv("KITTY_WINDOW_ID") != "" {
		return StyleKitty, "no probe answer; kitty env heuristic"
	}
	if strings.Contains(term, "ghostty") || strings.EqualFold(prog, "ghostty") ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return StyleKitty, "no probe answer; ghostty env heuristic"
	}
	return StyleBlocks, "no probe answer, no env match — universal fallback"
}

// kittyPlaceholders reports whether the terminal can be expected to implement
// the Unicode-placeholder half of the kitty graphics protocol (U=1 virtual
// placements bound to U+10EEEE cells), which renderPlaceholders requires.
//
// The a=q support query does NOT answer this. Konsole (≥ 25) implements kitty
// image transmission and answers a=q with ";OK", but has no placeholder
// support: it gives the image an ordinary placement at the cursor. The result
// is a crisp cover painted in the wrong place, orphaned there because nothing
// owns those cells — and duplicated on every re-placement and resize (MUS-29).
//
// There is no capability query for placeholders, so the only signal is the
// terminal naming itself. Konsole exports KONSOLE_VERSION; iTerm2 sets
// TERM_PROGRAM (and is caught earlier by DetectArtworkStyle anyway — listed
// here too so no future reordering can hand it placeholders). This is a
// denylist rather than an allowlist on purpose: env sniffing misses Ghostty
// on Linux, which is what made MUS-20 fall back to block art in the first
// place.
func kittyPlaceholders() bool {
	if os.Getenv("KONSOLE_VERSION") != "" || os.Getenv("KONSOLE_DBUS_SESSION") != "" {
		return false
	}
	if isITerm2() {
		return false
	}
	return true
}

// isITerm2 reports whether we are (or are sshed out of) iTerm2, which names
// itself in TERM_PROGRAM locally and forwards LC_TERMINAL over ssh.
//
// TERM_PROGRAM wins whenever it is set, and the two signals must NOT simply be
// OR'd: LC_TERMINAL is an LC_* variable, so it is built to survive into child
// processes — launch Ghostty from an iTerm2 shell and Ghostty inherits
// LC_TERMINAL=iTerm2 forever. It sets TERM_PROGRAM=ghostty for its own shell
// but cannot unset what it inherited, so an OR would hand Ghostty the iTerm2
// protocol and blank its artwork panel. Whatever set TERM_PROGRAM is the
// terminal actually on the other end of this tty; only when nothing did (ssh
// forwards LC_* but not TERM_PROGRAM) is the inherited hint worth trusting.
func isITerm2() bool {
	if prog := os.Getenv("TERM_PROGRAM"); prog != "" {
		return strings.EqualFold(prog, "iTerm.app")
	}
	return strings.EqualFold(os.Getenv("LC_TERMINAL"), "iTerm2")
}

// kittyImageID derives the kitty image id for a cover URL. Masked to 24 bits
// because the id rides in the placeholder cells' foreground color, which
// carries exactly 24; 0 is the protocol's "no id", so it maps to 1.
func kittyImageID(url string) uint32 {
	id := crc32.ChecksumIEEE([]byte(url)) & 0xFFFFFF
	if id == 0 {
		id = 1
	}
	return id
}

// kittyTxStats describes a transmit's actual size on the wire — for the
// artwork debug log and the probe, because a per-cover size or quota limit in
// the terminal presents as "some albums render, some don't" (MUS-34).
type kittyTxStats struct {
	PNGBytes int
	B64Chars int
	Chunks   int
}

// kittyTransmit encodes img as PNG and returns the chunked APC escape
// sequence that stores it in the terminal under the given image id.
//
// quiet is the protocol's q key. The app renders with q=2 — suppress the OK
// and any ERROR response alike, because a reply would land in Bubble Tea's
// input stream and be parsed as keystrokes — which means a terminal that
// rejects a cover rejects it silently (MUS-34). The artwork probe replays the
// same transmit with quiet=0 (the key is omitted; verbose is the default)
// outside Bubble Tea, precisely to hear that rejection.
func kittyTransmit(id uint32, img image.Image, quiet int) (string, kittyTxStats, error) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", kittyTxStats{}, err
	}
	payload := base64.StdEncoding.EncodeToString(pngBuf.Bytes())
	stats := kittyTxStats{PNGBytes: pngBuf.Len(), B64Chars: len(payload)}

	quietKey := ""
	if quiet != 0 {
		quietKey = fmt.Sprintf("q=%d,", quiet)
	}

	const chunkSize = 4096
	var b strings.Builder
	first := true
	for len(payload) > 0 {
		n := chunkSize
		if n > len(payload) {
			n = len(payload)
		}
		chunk := payload[:n]
		payload = payload[n:]
		more := 0
		if len(payload) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=t,%sf=100,i=%d,m=%d;%s\x1b\\", quietKey, id, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_G%sm=%d;%s\x1b\\", quietKey, more, chunk)
		}
		stats.Chunks++
	}
	return b.String(), stats, nil
}

// kittyPlacement creates (or replaces) the virtual placement binding image
// id to a cols×rows cell rectangle for Unicode-placeholder rendering.
// quiet as in kittyTransmit: the app passes 2, the probe 0.
func kittyPlacement(id uint32, cols, rows int, quiet int) string {
	quietKey := ""
	if quiet != 0 {
		quietKey = fmt.Sprintf("q=%d,", quiet)
	}
	return fmt.Sprintf("\x1b_Ga=p,%sU=1,i=%d,c=%d,r=%d\x1b\\", quietKey, id, cols, rows)
}

// kittyDelete frees the image data and any placements for id.
func kittyDelete(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,q=2,d=I,i=%d\x1b\\", id)
}

// kittyPlaceholderRow builds one row of the placeholder grid: `cols` cells of
// U+10EEEE, each tagged with its row/column diacritics, colored with the
// 24-bit image id so the terminal knows which image to draw.
//
// The foreground color carries the image id: 24 bits packed as R, G, B.
// We emit the SGR escape directly rather than routing through lipgloss →
// termenv → go-colorful, which converts hex → float64 via (1.0/255.0) →
// uint8 via (*255). Because 1/255 is not exactly representable in binary64,
// 24 of 256 byte values round-trip to b-1, corrupting the id emitted to the
// terminal and causing it to look up an image it never received (MUS-34).
func kittyPlaceholderRow(id uint32, row, cols int) string {
	r := (id >> 16) & 0xff
	g := (id >> 8) & 0xff
	b := id & 0xff
	var sb strings.Builder
	fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", r, g, b)
	for col := 0; col < cols; col++ {
		sb.WriteRune(placeholderRune)
		sb.WriteRune(rowColDiacritics[row])
		sb.WriteRune(rowColDiacritics[col])
	}
	sb.WriteString("\x1b[0m")
	return sb.String()
}
