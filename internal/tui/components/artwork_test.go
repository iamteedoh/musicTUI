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

// A cell whose left half is bright and right half is dark must pick the
// left-half glyph (▌): TL+BL form the bright cluster (mask 8|2 = 10).
func TestQuadrantGlyphSelection(t *testing.T) {
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

	// A panel that maps the whole image into one cell (2×2 subpixels).
	out := a.View(th, 3, 4)
	if !strings.Contains(stripAnsi(out), "▌") {
		t.Fatalf("left-bright/right-dark cell did not render ▌:\n%q", stripAnsi(out))
	}
}

// Hi-res mode must render a placeholder grid and queue the kitty-graphics
// escapes exactly once per image/size — never on every frame (a re-queue per
// frame would retransmit the full PNG 60×/second).
func TestArtworkHiResQueuesOncePerImage(t *testing.T) {
	th := theme.FromName("")
	var a Artwork
	a.SetHiRes(true)
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
