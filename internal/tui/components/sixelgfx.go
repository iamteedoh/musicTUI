package components

import (
	"bytes"
	"fmt"
	"image"
	"image/color"

	"github.com/mattn/go-sixel"
)

// Sixel graphics — real pixels for terminals that don't speak kitty.
//
// Unlike the kitty protocol there are no Unicode placeholders: a sixel image
// is painted at the cursor and occupies screen cells directly, with no text
// binding it to the frame. So we:
//
//  1. print blanks in the view where the image goes, giving Bubble Tea a
//     stable, unchanging block of cells it will not repaint, and
//  2. write the DCS payload out of band, cursor-positioned at the panel's
//     absolute origin, then restore the cursor.
//
// The escapes are queued in Artwork.oob and flushed by the app layer, exactly
// as the kitty payloads are. Because the artwork panel is memoized on its
// signature + geometry, the encode only runs when the cover or the panel size
// actually changes.
//
// Supported by Windows Terminal ≥ 1.22, WezTerm, xterm (-ti vt340), mintty,
// foot and Konsole. Detected via DA1 attribute 4 (see internal/termcap).

// sixelAt positions the cursor at a 1-based (row, col), writes the image, and
// restores the cursor so Bubble Tea's renderer resumes where it left off.
// DECSC/DECRC (ESC 7 / ESC 8) rather than CSI s/u: the former is what Windows
// Terminal and xterm both implement unconditionally.
func sixelAt(row, col int, payload string) string {
	return "\x1b7" + fmt.Sprintf("\x1b[%d;%dH", row, col) + payload + "\x1b8"
}

// sixelEncode box-filters img down to exactly pxW×pxH and encodes it as a
// self-contained DCS sixel sequence.
func sixelEncode(img image.Image, pxW, pxH int) (string, error) {
	if pxW <= 0 || pxH <= 0 {
		return "", fmt.Errorf("sixel: bad target size %dx%d", pxW, pxH)
	}
	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	// Photographs quantized to 256 colors band visibly in gradients; dithering
	// costs little here because the image is encoded once per track, not per
	// frame.
	enc.Dither = true
	if err := enc.Encode(scaleToRGBA(img, pxW, pxH)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// scaleToRGBA resizes img to w×h using the same box filter the character-art
// renderers use, so all three styles sample the cover identically.
func scaleToRGBA(img image.Image, w, h int) *image.RGBA {
	px := boxScale(img, w, h)
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := px[y][x]
			out.SetRGBA(x, y, color.RGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
	}
	return out
}
