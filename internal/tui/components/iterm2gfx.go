package components

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
)

// iTerm2 inline images — real pixels for iTerm2, which speaks neither of the
// dialects we already render (MUS-30).
//
// iTerm2 ≥ 3.6 answers the kitty a=q support query with ";OK", but — like
// Konsole — it does not render the Unicode-placeholder half of the protocol,
// so the kitty tier draws nothing there: a blank panel. Its DA1 reply also
// advertises sixel, but it reports no usable cell size (the CSI 14 t answer
// includes window padding, so the 14/18 division is inexact), which disables
// the sixel tier by design. What iTerm2 does implement, natively and since
// ~2.9, is its own inline-image protocol: OSC 1337 File=, documented at
// https://iterm2.com/documentation-images.html.
//
// Two properties make it a good fit here:
//
//   - The image is sized in CHARACTER CELLS, not pixels, so the unusable
//     cell-size report stops mattering entirely.
//   - Like sixel, the image is painted at the cursor — so it rides the
//     existing sixel plumbing unchanged: blank cells reserved in the view, a
//     cursor-positioned payload written out of band, and a repaint whenever
//     Bubble Tea rewrites a covered row. Unlike Konsole's sixel, overwriting
//     a cell in iTerm2 erases that cell's slice of the image, so a moved
//     panel can never orphan stale pixels.
//
// What it costs us, and why the encoding below is JPEG: the protocol has no
// image id, so the terminal caches nothing and the WHOLE cover goes back over
// the wire on every repaint.

// jpegQuality is the encode quality for the cover.
//
// 90 is the visually-lossless knee for photographs, and the terminal scales
// the cover down to a couple of hundred pixels anyway, so any artifact lands
// far below the resample floor. It is a payload-size knob first: see
// iterm2Encode.
const jpegQuality = 90

// iterm2Encode renders img as a base64 JPEG wrapped in iTerm2's OSC 1337
// File= sequence, sized to exactly cols×rows character cells.
//
// JPEG rather than PNG, and the difference is the whole ballgame. The cover is
// re-sent on every repaint — which Bubble Tea forces whenever it rewrites a
// line the image shares, and the lyrics sit on those same lines, so a single
// highlight moving to the next line costs a full retransmit. A 640×640 cover
// is ~1 MB as PNG: iTerm2 reads that in chunks and refreshes the screen
// between them, so the cover visibly blinks every time a lyric advances. The
// same cover is ~40 KB as JPEG — less than the sixel tier already re-sends per
// repaint — which puts the whole payload inside a single refresh and the blink
// disappears (MUS-30). Album covers are opaque photographs, so JPEG's missing
// alpha channel costs nothing.
//
// The image is sent at full source resolution: the protocol sizes it in cells,
// so the terminal does the scaling with every pixel we have (as the kitty tier
// also does), which keeps the cover sharp on retina displays — where a cell is
// twice as many device pixels as the terminal's own size reports admit.
//
// inline=1 displays the image instead of offering a download.
// preserveAspectRatio=0 fills the cell box exactly — the box was already
// fitted to the image's aspect with the same 1-wide × 2-tall cell assumption
// every other renderer uses, so letting iTerm2 letterbox a second time would
// only shrink the cover. doNotMoveCursor=1 keeps the cursor put (the payload
// is wrapped in DECSC/DECRC by the caller anyway, but this also rules out a
// scroll if the image ever touches the bottom row).
func iterm2Encode(img image.Image, cols, rows int) (string, error) {
	if cols <= 0 || rows <= 0 {
		return "", fmt.Errorf("iterm2: bad target size %dx%d", cols, rows)
	}
	var jpgBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", err
	}
	payload := base64.StdEncoding.EncodeToString(jpgBuf.Bytes())
	return fmt.Sprintf("\x1b]1337;File=inline=1;size=%d;width=%d;height=%d;preserveAspectRatio=0;doNotMoveCursor=1:%s\a",
		jpgBuf.Len(), cols, rows, payload), nil
}
