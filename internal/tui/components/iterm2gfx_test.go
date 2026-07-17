package components

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

// The encoder is the one piece we cannot eyeball outside iTerm2, so unwrap
// our own OSC and check the image inside actually survived.
func TestITerm2EncodeRoundTrip(t *testing.T) {
	want := color.RGBA{R: 220, G: 30, B: 90, A: 255}
	payload, err := iterm2Encode(solidImage(64, 48, want), 12, 6)
	if err != nil {
		t.Fatalf("iterm2Encode: %v", err)
	}

	const prefix = "\x1b]1337;File="
	if !strings.HasPrefix(payload, prefix) {
		t.Fatalf("payload does not start with OSC 1337 File=: %.20q", payload)
	}
	if !strings.HasSuffix(payload, "\a") {
		t.Fatalf("payload is not BEL-terminated: %.8q", payload[len(payload)-4:])
	}
	colon := strings.IndexByte(payload, ':')
	if colon < 0 {
		t.Fatal("no ':' separating File= arguments from the image data")
	}
	args := payload[len(prefix):colon]

	// inline=1 displays instead of downloading; width/height are in CELLS
	// (that is the point of this tier — no pixel cell size needed);
	// preserveAspectRatio=0 fills the box we already fitted to the image;
	// doNotMoveCursor=1 keeps the renderer's cursor untouched.
	for _, wantArg := range []string{
		"inline=1", "width=12", "height=6",
		"preserveAspectRatio=0", "doNotMoveCursor=1",
	} {
		if !containsArg(args, wantArg) {
			t.Errorf("File= arguments %q lack %q", args, wantArg)
		}
	}

	raw, err := base64.StdEncoding.DecodeString(payload[colon+1 : len(payload)-1])
	if err != nil {
		t.Fatalf("image data is not valid base64: %v", err)
	}
	if !containsArg(args, fmt.Sprintf("size=%d", len(raw))) {
		t.Errorf("size argument disagrees with the %d-byte payload: %q", len(raw), args)
	}
	img, err := jpeg.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("decode our own JPEG: %v", err)
	}
	// The full-resolution image is sent — iTerm2 does the scaling (like the
	// kitty tier), so retina displays get every pixel we have.
	if b := img.Bounds(); b.Dx() != 64 || b.Dy() != 48 {
		t.Fatalf("decoded size = %dx%d, want the 64x48 source", b.Dx(), b.Dy())
	}
	r, g, bl, _ := img.At(32, 24).RGBA()
	dr, dg, db := int(r>>8)-int(want.R), int(g>>8)-int(want.G), int(bl>>8)-int(want.B)
	if dr*dr+dg*dg+db*db > 3*8*8 { // lossy, but a flat color must stay flat
		t.Fatalf("center pixel = (%d,%d,%d), want ~(%d,%d,%d)",
			r>>8, g>>8, bl>>8, want.R, want.G, want.B)
	}

	if _, err := iterm2Encode(solidImage(4, 4, want), 0, 3); err == nil {
		t.Fatal("a zero-cell target must be rejected, not emitted")
	}
}

// photoImage is a stand-in for an album cover: smooth gradients plus
// structure. Flat color would flatter any encoder and noise would flatter
// none, so neither would size a payload honestly.
func photoImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			ring := math.Sin(math.Hypot(fx-0.5, fy-0.5) * 28)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(40 + 200*fx*(0.6+0.4*ring)),
				G: uint8(30 + 180*fy),
				B: uint8(90 + 120*math.Abs(ring)),
				A: 255,
			})
		}
	}
	return img
}

// The payload budget IS the feature. iTerm2's protocol has no image id, so the
// whole cover is re-sent on every repaint — and Bubble Tea forces one whenever
// it rewrites a line the image shares, which the lyrics do every time the
// highlight advances. At ~1 MB (a full-resolution PNG, the first cut of this
// tier) iTerm2 reads the payload in chunks and refreshes between them, so the
// cover visibly blinks on every lyric. JPEG puts it under the sixel tier's
// per-repaint payload, and the blink goes away (MUS-30).
//
// The ceiling is what guards that: it is far above JPEG's real cost (~45 KB
// for a 640×640 cover) and far below PNG's, so it cannot fail by accident —
// only by someone reaching for a lossless encoder again.
func TestITerm2PayloadStaysSmallEnoughNotToFlicker(t *testing.T) {
	// Spotify's largest cover art, which is what mapping.go asks for.
	payload, err := iterm2Encode(photoImage(640, 640), 28, 14)
	if err != nil {
		t.Fatalf("iterm2Encode: %v", err)
	}
	const budget = 150 << 10
	if len(payload) > budget {
		t.Fatalf("a 640x640 cover encodes to %d KB, over the %d KB per-repaint budget — "+
			"the artwork will blink whenever a lyric advances (MUS-30)",
			len(payload)>>10, budget>>10)
	}
	t.Logf("640x640 cover = %d KB on the wire per repaint", len(payload)>>10)
}

// containsArg reports whether the semicolon-separated File= argument list
// carries exactly this key=value pair (substring matching would let
// "width=12" pass for "width=120").
func containsArg(args, want string) bool {
	for _, a := range strings.Split(args, ";") {
		if a == want {
			return true
		}
	}
	return false
}

func newITerm2Artwork(originCol, originRow int) *Artwork {
	a := &Artwork{}
	a.style = StyleITerm2
	a.img = solidImage(300, 300, color.RGBA{R: 10, G: 200, B: 120, A: 255})
	a.imageURL = "https://example.invalid/cover.jpg"
	a.originCol, a.originRow = originCol, originRow
	return a
}

// primeITerm2 performs the encode the app layer normally does off the event
// loop, so the renderer can publish a draw.
func primeITerm2(t *testing.T, a *Artwork, w, h int) {
	t.Helper()
	a.renderITerm2(w, h) // records the geometry it needs
	work, ok := a.PendingSixel()
	if !ok {
		t.Fatal("renderITerm2 did not request an encode")
	}
	if work.Style != StyleITerm2 {
		t.Fatalf("pending work carries style %v, want StyleITerm2", work.Style)
	}
	payload, err := EncodeSixel(work)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if !strings.HasPrefix(payload, "\x1b]1337;File=") {
		t.Fatalf("EncodeSixel routed a StyleITerm2 work to the wrong encoder: %.20q", payload)
	}
	if !a.SetSixelPayload(work.URL, work.Cols, work.Rows, payload) {
		t.Fatal("SetSixelPayload rejected a payload for the geometry it asked for")
	}
}

// Guessing the origin would paint the cover over some other panel, so it must
// degrade to character art — but a missing CELL SIZE must not: not needing
// one is the reason this tier exists (MUS-30).
func TestRenderITerm2FallsBackOnlyWithoutOrigin(t *testing.T) {
	a := newITerm2Artwork(0, 0)
	got := a.renderITerm2(30, 15)
	want := newITerm2Artwork(0, 0).renderBlocks(30, 15)
	if got != want {
		t.Error("renderITerm2 did not fall back to blocks without an origin")
	}
	if seq, _, _ := a.SixelDraw(); seq != "" {
		t.Errorf("published a %d-byte draw despite an unknown origin", len(seq))
	}

	// With an origin but no cell size (iTerm2 reports none), it must render.
	a = newITerm2Artwork(61, 8)
	a.cellW, a.cellH = 0, 0
	if out := a.renderITerm2(30, 15); out == want {
		t.Fatal("renderITerm2 fell back to blocks despite a known origin")
	}
	if _, ok := a.PendingSixel(); !ok {
		t.Fatal("no encode requested despite a known origin")
	}
}

// The payload must be cursor-positioned at the panel origin (offset by the
// centering pad) and restore the cursor afterwards, or Bubble Tea's renderer
// resumes writing in the wrong place.
func TestRenderITerm2EmitsPositionedPayload(t *testing.T) {
	const originCol, originRow = 61, 8
	a := newITerm2Artwork(originCol, originRow)
	primeITerm2(t, a, 30, 15)

	view := a.renderITerm2(30, 15)
	oob, drawRow, drawRows := a.SixelDraw()
	if oob == "" {
		t.Fatal("no draw published")
	}
	if !strings.HasPrefix(oob, "\x1b7") || !strings.HasSuffix(oob, "\x1b8") {
		t.Fatal("payload is not wrapped in cursor save/restore")
	}
	if !strings.Contains(oob, "\x1b]1337;File=") {
		t.Fatal("no OSC 1337 payload in the published draw")
	}

	// Recompute the expected geometry the same way renderITerm2 does: cells
	// are 1×2 units, so a 300×300 cover in a 30×15 panel lands on 28×14.
	charW, charH := 30-2, 15
	scale := float64(300) / float64(charW)
	if s := float64(300) / float64(charH*2); s > scale {
		scale = s
	}
	cellsW := int(float64(300) / scale)
	cellsH := int(float64(300) / scale / 2)
	leftPad := (30 - cellsW) / 2
	topPad := (15 - cellsH) / 2

	wantCUP := fmt.Sprintf("\x1b[%d;%dH", originRow+topPad, originCol+leftPad)
	if !strings.Contains(oob, wantCUP) {
		t.Fatalf("payload not positioned at %q", wantCUP)
	}
	if drawRow != originRow+topPad || drawRows != cellsH {
		t.Fatalf("draw covers row %d over %d rows, want row %d over %d",
			drawRow, drawRows, originRow+topPad, cellsH)
	}

	// The blank cells the image sits on must match its cell footprint,
	// otherwise Bubble Tea repaints over the pixels.
	lines := strings.Split(view, "\n")
	gotTop := 0
	for gotTop < len(lines) && lines[gotTop] == "" {
		gotTop++
	}
	imgLines := lines[gotTop:]
	if gotTop != topPad || len(imgLines) != cellsH {
		t.Fatalf("view blanks %d rows after %d pad rows, want %d after %d",
			len(imgLines), gotTop, cellsH, topPad)
	}
	if got := len(imgLines[0]); got != leftPad+cellsW {
		t.Fatalf("blank row is %d cells wide, want %d", got, leftPad+cellsW)
	}
	if strings.TrimSpace(imgLines[0]) != "" {
		t.Fatal("image rows must be blank — any glyph would overwrite the pixels")
	}
}

// A cover that is no longer on screen must not keep being repainted over the
// "Loading..." / "♫" placeholder that replaced it — through View, which also
// exercises the StyleITerm2 wiring in the style switch.
func TestITerm2DrawClearedWhenNoCover(t *testing.T) {
	a := newITerm2Artwork(61, 8)
	a.albumName, a.artist = "Album", "Artist"
	_ = a.View(theme.Nord(), 30, 18) // records the geometry
	primeITerm2(t, a, 30, 15)
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
