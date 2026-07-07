package components

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

// checkerImage builds a w×h image whose left half is `left` and right half
// is `right`, for exercising the scaler with known colors.
func fillImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// boxScale must average all covered source pixels: a half-white/half-black
// source collapsed to a single pixel is mid-gray, not whichever pixel a
// nearest-neighbor sample would have hit.
func TestBoxScaleAverages(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{255, 255, 255, 255})
	img.Set(0, 1, color.RGBA{0, 0, 0, 255})
	img.Set(1, 1, color.RGBA{0, 0, 0, 255})

	out := boxScale(img, 1, 1)
	got := out[0][0]
	if got.R != 127 || got.G != 127 || got.B != 127 {
		t.Fatalf("boxScale(2x2 half white/black -> 1x1) = %+v, want ~{127 127 127}", got)
	}
}

// Every artwork cell must be painted — dark pixels included. The old braille
// renderer emitted a bare space for dark regions, which read as holes in the
// cover; the quadrant renderer paints every cell (a uniform cell renders as
// a full block in its own color) — MUS-15.
func TestArtworkRendersSolidCells(t *testing.T) {
	th := theme.FromName("")
	var a Artwork
	a.LoadURL("http://example/x.jpg")
	// Pure black cover: worst case for the old renderer (all dots off).
	a.SetFullImage(fillImage(64, 64, color.RGBA{0, 0, 0, 255}), "http://example/x.jpg")
	a.SetAlbumInfo("Album", "Artist")

	out := a.View(th, 20, 14)
	plain := stripAnsi(out)
	blocks := strings.Count(plain, "█")
	if blocks < 50 {
		t.Fatalf("expected a solidly painted cover (>=50 full-block cells), got %d:\n%q", blocks, plain)
	}
}

// A cell whose left column is bright and right column dark must pick the
// left-column braille glyph ⡇ (dots 1,2,3,7 → mask 0x47) with the bright
// cluster as foreground.
func TestBrailleGlyphSelection(t *testing.T) {
	th := theme.FromName("")
	img := image.NewRGBA(image.Rect(0, 0, 2, 4))
	for y := 0; y < 4; y++ {
		img.Set(0, y, color.RGBA{255, 255, 255, 255})
		img.Set(1, y, color.RGBA{0, 0, 0, 255})
	}

	var a Artwork
	a.SetStyle(StyleBraille)
	a.LoadURL("u")
	a.SetFullImage(img, "u")
	a.SetAlbumInfo("A", "B")

	// A panel that maps the whole image into one cell (2×4 subpixels).
	out := a.View(th, 3, 4)
	if !strings.Contains(stripAnsi(out), "⡇") {
		t.Fatalf("left-bright/right-dark cell did not render ⡇:\n%q", stripAnsi(out))
	}
}

// The default block renderer must pick the partition with least color error:
// a left-white/right-black cell is exactly the left-half glyph ▌ (mask 1010).
func TestBlocksGlyphSelection(t *testing.T) {
	th := theme.FromName("")
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 255, 255, 255})
	img.Set(0, 1, color.RGBA{255, 255, 255, 255})
	img.Set(1, 0, color.RGBA{0, 0, 0, 255})
	img.Set(1, 1, color.RGBA{0, 0, 0, 255})

	var a Artwork
	a.LoadURL("u")
	a.SetFullImage(img, "u")
	a.SetAlbumInfo("A", "B")

	out := a.View(th, 3, 4)
	if !strings.Contains(stripAnsi(out), "▌") {
		t.Fatalf("left-bright/right-dark cell did not render ▌:\n%q", stripAnsi(out))
	}
}

// Regression for the resize crash: uniform (and near-uniform) covers used to
// hit an integer divide-by-zero in the braille renderer when floating-point
// rounding left the bright cluster empty. Render flat and noisy covers at
// many panel sizes in both character styles — must not panic.
func TestArtworkNoPanicAcrossSizes(t *testing.T) {
	th := theme.FromName("")
	imgs := []image.Image{
		fillImage(64, 64, color.RGBA{37, 37, 37, 255}),
		fillImage(640, 640, color.RGBA{255, 255, 255, 255}),
		func() image.Image { // subtle 1-value noise: the ULP trap
			img := image.NewRGBA(image.Rect(0, 0, 64, 64))
			for y := 0; y < 64; y++ {
				for x := 0; x < 64; x++ {
					v := uint8(100 + (x+y)%2)
					img.Set(x, y, color.RGBA{v, v, v, 255})
				}
			}
			return img
		}(),
	}
	for _, style := range []ArtworkStyle{StyleBlocks, StyleBraille} {
		for _, im := range imgs {
			for _, dim := range [][2]int{{3, 4}, {20, 14}, {80, 50}, {200, 120}} {
				var a Artwork
				a.SetStyle(style)
				a.LoadURL("u")
				a.SetFullImage(im, "u")
				a.SetAlbumInfo("A", "B")
				_ = a.View(th, dim[0], dim[1]) // must not panic
			}
		}
	}
}

// Hi-res mode must render a placeholder grid and queue the kitty-graphics
// escapes exactly once per image/size — never on every frame (a re-queue per
// frame would retransmit the full PNG 60×/second).
func TestArtworkHiResQueuesOncePerImage(t *testing.T) {
	th := theme.FromName("")
	var a Artwork
	a.SetStyle(StyleKitty)
	a.LoadURL("u1")
	a.SetFullImage(fillImage(64, 64, color.RGBA{50, 90, 200, 255}), "u1")
	a.SetAlbumInfo("A", "B")

	out := a.View(th, 20, 14)
	if !strings.ContainsRune(out, placeholderRune) {
		t.Fatalf("hi-res render has no U+10EEEE placeholder cells")
	}

	oob := a.TakeOOB()
	if !strings.Contains(oob, "\x1b_Ga=t,") || !strings.Contains(oob, "a=p,") {
		t.Fatalf("first render did not queue transmit+placement: %q", oob[:min(len(oob), 120)])
	}

	// Same image, same size: nothing new to send.
	_ = a.View(th, 20, 14)
	if again := a.TakeOOB(); again != "" {
		t.Fatalf("unchanged render re-queued escapes (would retransmit every frame): %q", again[:min(len(again), 120)])
	}

	// Size change: only a (tiny) re-placement, not a retransmit.
	_ = a.View(th, 30, 20)
	rePlace := a.TakeOOB()
	if !strings.Contains(rePlace, "a=p,") {
		t.Fatalf("resize did not re-place the image")
	}
	if strings.Contains(rePlace, "a=t,") {
		t.Fatalf("resize retransmitted the whole image")
	}
}

// A square cover must stay square-ish: rendered rows ≈ cells wide / 2 ratio
// held by the 1×2 pixel-per-cell mapping, and it must fit the given box.
func TestArtworkFitsPanel(t *testing.T) {
	th := theme.FromName("")
	var a Artwork
	a.LoadURL("u")
	a.SetFullImage(fillImage(640, 640, color.RGBA{10, 200, 90, 255}), "u")
	a.SetAlbumInfo("A", "B")

	width, height := 30, 20
	out := a.View(th, width, height)
	for i, line := range strings.Split(out, "\n") {
		if w := len([]rune(stripAnsi(line))); w > width {
			t.Fatalf("line %d wider than panel: %d > %d", i, w, width)
		}
	}
}
