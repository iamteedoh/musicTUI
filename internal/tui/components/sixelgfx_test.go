package components

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
	"github.com/mattn/go-sixel"
)

func solidImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

// The encoder is the one piece we cannot eyeball from a non-sixel terminal, so
// decode our own payload back and check the pixels actually survived.
func TestSixelEncodeRoundTrip(t *testing.T) {
	want := color.RGBA{R: 220, G: 30, B: 90, A: 255}
	payload, err := sixelEncode(solidImage(64, 64, want), 40, 20)
	if err != nil {
		t.Fatalf("sixelEncode: %v", err)
	}
	// P2=1. Sixel paints in bands of 6 pixel rows; when the height isn't a
	// multiple of 6 the final band still covers all six, and P2=0 would fill
	// those leftover rows with the background color — a dark stripe along the
	// bottom of every cover. P2=1 leaves untouched pixels alone (MUS-29).
	if !strings.HasPrefix(payload, "\x1bP0;1;8q") {
		t.Fatalf("payload must be P2=1 (transparent), got %.10q", payload)
	}
	if !strings.HasSuffix(payload, "\x1b\\") {
		t.Fatalf("payload is not ST-terminated: %.8q", payload[len(payload)-4:])
	}

	var got image.Image
	if err := sixel.NewDecoder(strings.NewReader(payload)).Decode(&got); err != nil {
		t.Fatalf("decode our own payload: %v", err)
	}
	b := got.Bounds()
	if b.Dx() < 40 || b.Dy() < 20 {
		t.Fatalf("decoded size = %dx%d, want at least 40x20", b.Dx(), b.Dy())
	}

	r, g, bl, _ := got.At(20, 10).RGBA()
	dr := int(r>>8) - int(want.R)
	dg := int(g>>8) - int(want.G)
	db := int(bl>>8) - int(want.B)
	// Dithered 256-color quantization of a flat color should land very close.
	if dr*dr+dg*dg+db*db > 3*20*20 {
		t.Fatalf("center pixel = (%d,%d,%d), want ~(%d,%d,%d)",
			r>>8, g>>8, bl>>8, want.R, want.G, want.B)
	}
}

// primeSixel performs the encode the app layer normally does off the event loop,
// so the renderer can publish a draw.
func primeSixel(t *testing.T, a *Artwork, w, h int) {
	t.Helper()
	a.renderSixel(w, h) // records the geometry it needs
	work, ok := a.PendingSixel()
	if !ok {
		t.Fatal("renderSixel did not request an encode")
	}
	payload, err := EncodeSixel(work)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if !a.SetSixelPayload(work.URL, work.Cols, work.Rows, payload) {
		t.Fatal("SetSixelPayload rejected a payload for the geometry it asked for")
	}
}

func newSixelArtwork(cellW, cellH, originCol, originRow int) *Artwork {
	a := &Artwork{}
	a.style = StyleSixel
	a.img = solidImage(300, 300, color.RGBA{R: 10, G: 200, B: 120, A: 255})
	a.imageURL = "https://example.invalid/cover.jpg"
	a.cellW, a.cellH = cellW, cellH
	a.originCol, a.originRow = originCol, originRow
	return a
}

// Guessing the cell size would overflow or under-fill the panel, and guessing
// the origin would paint the cover over some other panel. Both must degrade to
// character art instead — silently and with no escape bytes emitted.
func TestRenderSixelFallsBackWhenGeometryUnknown(t *testing.T) {
	cases := []struct {
		name                               string
		cellW, cellH, originCol, originRow int
	}{
		{"no cell size", 0, 0, 60, 8},
		{"no cell width", 0, 20, 60, 8},
		{"no origin", 10, 20, 0, 0},
		{"no origin row", 10, 20, 60, 0},
	}
	for _, c := range cases {
		a := newSixelArtwork(c.cellW, c.cellH, c.originCol, c.originRow)
		got := a.renderSixel(30, 15)

		blocks := newSixelArtwork(c.cellW, c.cellH, c.originCol, c.originRow)
		want := blocks.renderBlocks(30, 15)

		if got != want {
			t.Errorf("%s: renderSixel did not fall back to blocks", c.name)
		}
		if seq, _, _ := a.SixelDraw(); seq != "" {
			t.Errorf("%s: published a %d-byte draw despite unknown geometry", c.name, len(seq))
		}
		if oob := a.TakeOOB(); oob != "" {
			t.Errorf("%s: emitted %d bytes of escapes despite unknown geometry", c.name, len(oob))
		}
	}
}

// A cover that is no longer on screen must not keep being repainted over the
// "Loading..." / "♫" placeholder that replaced it.
func TestSixelDrawClearedWhenNoCover(t *testing.T) {
	a := newSixelArtwork(10, 20, 61, 8)
	a.albumName, a.artist = "Album", "Artist"
	_ = a.View(theme.Nord(), 30, 18) // records the geometry
	primeSixel(t, a, 30, 15)
	_ = a.View(theme.Nord(), 30, 18)
	if seq, _, _ := a.SixelDraw(); seq == "" {
		t.Fatal("no draw published for a loaded cover")
	}

	a.img = nil
	a.loading = true
	_ = a.View(theme.Nord(), 30, 18)
	if seq, row, rows := a.SixelDraw(); seq != "" || row != 0 || rows != 0 {
		t.Fatalf("draw survived the cover going away: seq=%d row=%d rows=%d", len(seq), row, rows)
	}
}

// The payload must be cursor-positioned at the panel origin (offset by the
// centering pad) and restore the cursor afterwards, or Bubble Tea's renderer
// resumes writing in the wrong place.
func TestRenderSixelEmitsPositionedPayload(t *testing.T) {
	const cellW, cellH = 10, 20
	const originCol, originRow = 61, 8
	a := newSixelArtwork(cellW, cellH, originCol, originRow)
	primeSixel(t, a, 30, 15)

	view := a.renderSixel(30, 15)
	oob, drawRow, drawRows := a.SixelDraw()
	if oob == "" {
		t.Fatal("no draw published")
	}
	if !strings.HasPrefix(oob, "\x1b7") || !strings.HasSuffix(oob, "\x1b8") {
		t.Fatal("payload is not wrapped in cursor save/restore")
	}
	if !strings.Contains(oob, "\x1bP") {
		t.Fatal("no DCS sixel payload in the published draw")
	}
	if drawRow <= 0 || drawRows <= 0 {
		t.Fatalf("draw rows not published: row=%d rows=%d", drawRow, drawRows)
	}

	// The image is centered, so derive the pad from the blank view it returned.
	lines := strings.Split(view, "\n")
	topPad := 0
	for topPad < len(lines) && lines[topPad] == "" {
		topPad++
	}
	imgLines := lines[topPad:]
	if len(imgLines) == 0 {
		t.Fatal("no image rows in the view")
	}

	// Recompute the expected geometry the same way renderSixel does.
	charW, charH := 30-2, 15
	boxW, boxH := charW*cellW, charH*cellH
	scale := float64(300) / float64(boxW)
	if s := float64(300) / float64(boxH); s > scale {
		scale = s
	}
	cellsW := int(float64(300)/scale) / cellW
	cellsH := int(float64(300)/scale) / cellH
	leftPad := (30 - cellsW) / 2
	wantTop := (15 - cellsH) / 2

	wantCUP := fmt.Sprintf("\x1b[%d;%dH", originRow+wantTop, originCol+leftPad)
	if !strings.Contains(oob, wantCUP) {
		t.Fatalf("payload not positioned at %q", wantCUP)
	}

	// The blank cells the image sits on must match the image's cell footprint,
	// otherwise Bubble Tea repaints over the pixels.
	if len(imgLines) != cellsH {
		t.Fatalf("view has %d image rows, want %d", len(imgLines), cellsH)
	}
	if got := len(imgLines[0]); got != leftPad+cellsW {
		t.Fatalf("blank row is %d cells wide, want %d", got, leftPad+cellsW)
	}
	if strings.TrimSpace(imgLines[0]) != "" {
		t.Fatal("image rows must be blank — any glyph would overwrite the pixels")
	}
}

// The published draw is stable across renders: the app decides when to repaint,
// based on whether the rows it covers were rewritten.
func TestRenderSixelPublishesStableDraw(t *testing.T) {
	a := newSixelArtwork(10, 20, 61, 8)
	primeSixel(t, a, 30, 15)

	a.renderSixel(30, 15)
	first, row1, rows1 := a.SixelDraw()
	if first == "" {
		t.Fatal("first render published nothing")
	}

	a.renderSixel(30, 15)
	second, row2, rows2 := a.SixelDraw()
	if second != first || row1 != row2 || rows1 != rows2 {
		t.Fatal("draw changed across identical renders")
	}

	// The rows must match the blank cells the view reserves for the image.
	view := a.renderSixel(30, 15)
	lines := strings.Split(view, "\n")
	topPad := 0
	for topPad < len(lines) && lines[topPad] == "" {
		topPad++
	}
	if got := len(lines) - topPad; got != rows1 {
		t.Fatalf("draw covers %d rows but the view blanks %d", rows1, got)
	}
	if _, row, _ := a.SixelDraw(); row != 8+topPad {
		t.Fatalf("draw starts at row %d, want origin(8)+topPad(%d)", row, topPad)
	}
}

// When the panel moves, the next render must paint at the new coordinates —
// otherwise the cover stays where the panel used to be.
func TestSetOriginMovesTheNextPayload(t *testing.T) {
	a := newSixelArtwork(10, 20, 61, 8)
	primeSixel(t, a, 30, 15)
	a.renderSixel(30, 15)
	before, _, _ := a.SixelDraw()

	a.SetOrigin(71, 9)
	a.renderSixel(30, 15)
	after, afterRow, _ := a.SixelDraw()

	if before == after {
		t.Fatal("payload position did not change after SetOrigin")
	}
	if afterRow < 9 {
		t.Fatalf("draw row = %d, want >= the new origin row 9", afterRow)
	}
	if !strings.Contains(after, "\x1b[9;") {
		t.Fatalf("payload not repositioned to row 9: %.24q", after)
	}
}

// Styles other than sixel must not queue any DCS payload.
func TestNonSixelStylesEmitNoDCS(t *testing.T) {
	for _, st := range []ArtworkStyle{StyleBlocks, StyleBraille} {
		a := newSixelArtwork(10, 20, 61, 8)
		a.style = st
		a.albumName, a.artist = "Album", "Artist"
		_ = a.View(theme.Nord(), 30, 18)
		if oob := a.TakeOOB(); strings.Contains(oob, "\x1bP") {
			t.Fatalf("style %v emitted a sixel payload", st)
		}
	}
}
