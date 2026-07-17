package components

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	switch strings.ToLower(os.Getenv("MUSICTUI_ARTWORK")) {
	case "blocks":
		return StyleBlocks
	case "braille":
		return StyleBraille
	case "kitty", "hires":
		return StyleKitty
	case "sixel":
		return StyleSixel
	case "iterm2", "iterm":
		return StyleITerm2
	}
	// Inside tmux the APC/DCS escapes would need passthrough wrapping; use blocks.
	if os.Getenv("TMUX") != "" {
		return StyleBlocks
	}
	// iTerm2 must be identified before the kitty probe is consulted: it
	// answers a=q with ";OK" (≥ 3.6) but renders no Unicode placeholders, so
	// trusting the probe leaves a blank panel (MUS-30). Its native inline
	// protocol is sized in cells, so it needs no probe result at all. Locally
	// iTerm2 always sets TERM_PROGRAM; over ssh it forwards LC_TERMINAL when
	// the server accepts LC_* (a stripped environment falls through to the
	// probe and, like Konsole over ssh, cannot be told apart from kitty).
	if isITerm2() {
		return StyleITerm2
	}
	// Authoritative: the terminal told us it supports kitty graphics — but only
	// our renderer's dialect of it counts (see kittyPlaceholders).
	if kittyProbe && kittyPlaceholders() {
		return StyleKitty
	}
	// Next best real-pixel path. Preferred over kitty's env heuristic below
	// because it, too, came from the terminal itself rather than a guess.
	if sixelProbe {
		return StyleSixel
	}
	// Fallback heuristic when the probe couldn't run (redirected output, etc.).
	term := os.Getenv("TERM")
	prog := os.Getenv("TERM_PROGRAM")
	if !kittyPlaceholders() {
		return StyleBlocks
	}
	if strings.Contains(term, "kitty") || os.Getenv("KITTY_WINDOW_ID") != "" {
		return StyleKitty
	}
	if strings.Contains(term, "ghostty") || strings.EqualFold(prog, "ghostty") ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return StyleKitty
	}
	return StyleBlocks
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

// kittyTransmit encodes img as PNG and returns the chunked APC escape
// sequence that stores it in the terminal under the given image id.
// q=2 suppresses terminal responses (they would land in Bubble Tea's input).
func kittyTransmit(id uint32, img image.Image) (string, error) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return "", err
	}
	payload := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

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
			fmt.Fprintf(&b, "\x1b_Ga=t,q=2,f=100,i=%d,m=%d;%s\x1b\\", id, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Gq=2,m=%d;%s\x1b\\", more, chunk)
		}
	}
	return b.String(), nil
}

// kittyPlacement creates (or replaces) the virtual placement binding image
// id to a cols×rows cell rectangle for Unicode-placeholder rendering.
func kittyPlacement(id uint32, cols, rows int) string {
	return fmt.Sprintf("\x1b_Ga=p,q=2,U=1,i=%d,c=%d,r=%d\x1b\\", id, cols, rows)
}

// kittyDelete frees the image data and any placements for id.
func kittyDelete(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=d,q=2,d=I,i=%d\x1b\\", id)
}

// kittyPlaceholderRow builds one row of the placeholder grid: `cols` cells of
// U+10EEEE, each tagged with its row/column diacritics, colored with the
// 24-bit image id so the terminal knows which image to draw.
func kittyPlaceholderRow(id uint32, row, cols int) string {
	fg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", (id>>16)&0xff, (id>>8)&0xff, id&0xff))
	var b strings.Builder
	for col := 0; col < cols; col++ {
		b.WriteRune(placeholderRune)
		b.WriteRune(rowColDiacritics[row])
		b.WriteRune(rowColDiacritics[col])
	}
	return lipgloss.NewStyle().Foreground(fg).Render(b.String())
}
