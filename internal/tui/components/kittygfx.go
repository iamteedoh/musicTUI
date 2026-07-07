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

// KittyGraphicsSupported reports whether the terminal is known to render
// Unicode-placeholder graphics. MUSICTUI_ARTWORK overrides detection:
// "blocks" forces the character-art fallback, "kitty" forces hi-res on.
func KittyGraphicsSupported() bool {
	switch strings.ToLower(os.Getenv("MUSICTUI_ARTWORK")) {
	case "blocks":
		return false
	case "kitty", "hires":
		return true
	}
	// Inside tmux the APC escapes would need passthrough wrapping; use blocks.
	if os.Getenv("TMUX") != "" {
		return false
	}
	term := os.Getenv("TERM")
	prog := os.Getenv("TERM_PROGRAM")
	if strings.Contains(term, "kitty") || os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}
	if strings.Contains(term, "ghostty") || strings.EqualFold(prog, "ghostty") ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}
	return false
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
