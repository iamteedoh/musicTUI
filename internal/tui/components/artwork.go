package components

import (
	"fmt"
	"hash/crc32"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type Artwork struct {
	mu        sync.RWMutex
	imageURL  string
	img       image.Image
	loading   bool
	err       string
	albumName string
	artist    string

	// Kitty-graphics (hi-res) state. When hiRes is set the panel renders a
	// Unicode-placeholder grid and queues the protocol escapes (transmit /
	// placement / delete) in oob; the app layer flushes that queue straight
	// to the terminal between frames (it must bypass Bubble Tea's diffing).
	hiRes          bool
	kittyID        uint32
	transmittedURL string // image URL already transmitted under kittyID
	placedCols     int
	placedRows     int
	oob            strings.Builder
}

type rgbColor struct{ R, G, B uint8 }
type ArtworkResult struct {
	Img    image.Image
	Pixels [][]rgbColor
	Gray   [][]uint8
	W, H   int
	URL    string
	Err    string
}

func NewArtwork() Artwork { return Artwork{} }

func (a *Artwork) LoadURL(url string) {
	if url == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if url == a.imageURL && a.img != nil {
		return
	}
	a.imageURL = url
	a.loading = true
	a.err = ""
	a.img = nil
}

func (a *Artwork) SetError(err string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.err = err
	a.loading = false
}

func (a *Artwork) SetFullImage(img image.Image, url string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if url != a.imageURL {
		return
	}
	a.img = img
	a.loading = false
}

func (a *Artwork) SetAlbumInfo(album, artist string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.albumName = album
	a.artist = artist
}

func (a *Artwork) SetImage(_ [][]rgbColor, _, _ int, _ string) {}
func (a *Artwork) SetGray(_ [][]uint8, _, _ int, _ string)     {}
func (a *Artwork) SetTrackInfo(_, _, _ string)                 {}

// SetHiRes selects the kitty-graphics renderer (true pixel artwork) instead
// of quadrant blocks. Call once at startup after terminal detection.
func (a *Artwork) SetHiRes(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.hiRes = v
}

// TakeOOB drains the queued kitty-protocol escapes. The app layer writes
// them directly to the terminal (out of band of Bubble Tea's renderer) —
// they carry image data, not visible text, so they must not be diffed,
// cached, or truncated like view content.
func (a *Artwork) TakeOOB() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.oob.String()
	a.oob.Reset()
	return s
}

func FetchArtwork(url string) ArtworkResult {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ArtworkResult{URL: url, Err: fmt.Sprintf("fetch: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ArtworkResult{URL: url, Err: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return ArtworkResult{URL: url, Err: fmt.Sprintf("decode: %v", err)}
	}
	return ArtworkResult{Img: img, URL: url}
}

// Signature returns a string that changes whenever View's output would change,
// for cache keying. It captures the async load transition (loading → img), so a
// cached render is replaced the moment the album art finishes downloading.
func (a *Artwork) Signature() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	loaded := '0'
	if a.img != nil {
		loaded = '1'
	}
	ld := '0'
	if a.loading {
		ld = '1'
	}
	return string(ld) + string(loaded) + "|" + a.imageURL + "|" + a.err + "|" + a.albumName + "|" + a.artist
}

func (a *Artwork) View(th theme.Theme, width, height int) string {
	// Full lock: the hi-res path queues protocol escapes as a side effect.
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.loading {
		return cText(lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Loading..."), width, height)
	}
	if a.err != "" {
		return cText(lipgloss.NewStyle().Foreground(th.Error).Render(a.err), width, height)
	}
	if a.img == nil {
		return cText(lipgloss.NewStyle().Foreground(th.Accent).Render("♫"), width, height)
	}

	// Reserve bottom 3 lines for album info
	infoLines := 3
	imgHeight := height - infoLines
	if imgHeight < 3 {
		imgHeight = 3
	}

	var imgStr string
	if a.hiRes {
		imgStr = a.renderPlaceholders(width, imgHeight)
	} else {
		imgStr = a.renderQuadrants(width, imgHeight)
	}

	// Album info text
	maxW := width - 2
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)

	album := a.albumName
	if len(album) > maxW {
		album = album[:maxW-1] + "…"
	}
	artist := a.artist
	if len(artist) > maxW {
		artist = artist[:maxW-1] + "…"
	}

	info := centerLn(accent.Render(album), width) + "\n" + centerLn(dim.Render(artist), width)

	return imgStr + "\n" + info
}

// renderPlaceholders draws the artwork as a kitty-graphics Unicode
// placeholder grid: real pixels, rendered by the terminal itself. Queues the
// transmit/placement escapes in a.oob when the image or grid size changes
// (flushed out-of-band by the app layer). Falls back to quadrant blocks if
// PNG encoding fails. Caller must hold a.mu.
func (a *Artwork) renderPlaceholders(width, height int) string {
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Same 1-wide × 2-tall cell aspect math as the block renderer, capped by
	// the diacritic table so every cell stays addressable.
	charW := width - 2
	charH := height
	scale := float64(srcW) / float64(charW)
	if s := float64(srcH) / float64(charH*2); s > scale {
		scale = s
	}
	cellsW := int(float64(srcW) / scale)
	cellsH := int(float64(srcH) / scale / 2)
	if cellsW < 1 {
		cellsW = 1
	}
	if cellsH < 1 {
		cellsH = 1
	}
	if max := maxKittyGridDim(); cellsW > max {
		cellsW = max
	}
	if max := maxKittyGridDim(); cellsH > max {
		cellsH = max
	}

	id := crc32.ChecksumIEEE([]byte(a.imageURL)) & 0xFFFFFF
	if id == 0 {
		id = 1
	}

	// Reconcile terminal-side state: transmit on a new image, re-place on a
	// grid-size change. Cache hits in the app's view cache skip this whole
	// function, so escapes are only queued when something actually changed.
	if a.transmittedURL != a.imageURL {
		if a.kittyID != 0 && a.kittyID != id {
			a.oob.WriteString(kittyDelete(a.kittyID))
		}
		tx, err := kittyTransmit(id, a.img)
		if err != nil {
			return a.renderQuadrants(width, height)
		}
		a.oob.WriteString(tx)
		a.oob.WriteString(kittyPlacement(id, cellsW, cellsH))
		a.kittyID = id
		a.transmittedURL = a.imageURL
		a.placedCols, a.placedRows = cellsW, cellsH
	} else if a.placedCols != cellsW || a.placedRows != cellsH {
		a.oob.WriteString(kittyPlacement(id, cellsW, cellsH))
		a.placedCols, a.placedRows = cellsW, cellsH
	}

	leftPad := (width - cellsW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - cellsH) / 2
	if topPad < 0 {
		topPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	for r := 0; r < cellsH; r++ {
		rows = append(rows, padStr+kittyPlaceholderRow(id, r, cellsW))
	}
	return strings.Join(rows, "\n")
}

// quadrantChars maps a 4-bit "bright subpixel" mask (TL=8, TR=4, BL=2, BR=1)
// to the block element whose painted quadrants match the bright cluster.
var quadrantChars = [16]string{
	" ", "▗", "▖", "▄", "▝", "▐", "▞", "▟",
	"▘", "▚", "▌", "▙", "▀", "▜", "▛", "█",
}

// renderQuadrants draws the artwork with quadrant block elements
// (▘▝▖▗▀▄▌▐▞▚…█): each character cell carries a 2×2 grid of image pixels.
// The four subpixels are split into a bright and a dark cluster by
// luminance; the glyph paints the bright cluster in the foreground color
// and the terminal background color fills the dark cluster. This keeps the
// spatial detail the old braille dot-matrix had (multiple subpixels per
// cell) while painting every cell solidly in two true colors — no dither
// noise, no gaps (chafa-style "symbols" rendering). Downscaling box-filters
// the source to avoid sampling noise.
func (a *Artwork) renderQuadrants(width, height int) string {
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Terminal cells are ~1:2 (w:h). Working in "units" where a cell is
	// 1 wide × 2 tall, fit the image into the panel box, then split every
	// cell into 2×2 subpixels (each 0.5 wide × 1 tall — a 1:2 region of
	// the source, which boxScale handles).
	charW := width - 2
	charH := height
	unitW := float64(charW)
	unitH := float64(charH * 2)

	scale := float64(srcW) / unitW
	if s := float64(srcH) / unitH; s > scale {
		scale = s
	}
	cellsW := int(float64(srcW) / scale)
	cellsH := int(float64(srcH) / scale / 2)
	if cellsW < 1 {
		cellsW = 1
	}
	if cellsH < 1 {
		cellsH = 1
	}

	// 2×2 subpixels per cell.
	px := boxScale(a.img, cellsW*2, cellsH*2)

	leftPad := (width - cellsW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - cellsH) / 2
	if topPad < 0 {
		topPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	// Styles repeat heavily across cells (flat color areas); cache them.
	styles := make(map[uint64]lipgloss.Style)
	styleFor := func(fg, bg rgbColor) lipgloss.Style {
		key := uint64(fg.R)<<40 | uint64(fg.G)<<32 | uint64(fg.B)<<24 |
			uint64(bg.R)<<16 | uint64(bg.G)<<8 | uint64(bg.B)
		st, ok := styles[key]
		if !ok {
			st = lipgloss.NewStyle().
				Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", fg.R, fg.G, fg.B))).
				Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", bg.R, bg.G, bg.B)))
			styles[key] = st
		}
		return st
	}

	lum := func(c rgbColor) float64 {
		return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
	}

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	for cy := 0; cy < cellsH; cy++ {
		var line strings.Builder
		line.WriteString(padStr)
		for cx := 0; cx < cellsW; cx++ {
			// The cell's 2×2 subpixels, in TL, TR, BL, BR order.
			quad := [4]rgbColor{
				px[cy*2][cx*2], px[cy*2][cx*2+1],
				px[cy*2+1][cx*2], px[cy*2+1][cx*2+1],
			}

			// Split into bright/dark clusters around the mean luminance.
			// The brightest subpixel always meets a >=-mean threshold, so
			// the mask is never 0 — every cell paints at least one quadrant.
			var mean float64
			for _, c := range quad {
				mean += lum(c)
			}
			mean /= 4

			var mask int
			var fgSum, bgSum [3]uint64
			var fgN, bgN uint64
			for i, c := range quad {
				if lum(c) >= mean {
					mask |= 8 >> i
					fgSum[0] += uint64(c.R)
					fgSum[1] += uint64(c.G)
					fgSum[2] += uint64(c.B)
					fgN++
				} else {
					bgSum[0] += uint64(c.R)
					bgSum[1] += uint64(c.G)
					bgSum[2] += uint64(c.B)
					bgN++
				}
			}

			fg := rgbColor{uint8(fgSum[0] / fgN), uint8(fgSum[1] / fgN), uint8(fgSum[2] / fgN)}
			bg := fg // uniform cell (mask 15): bg unused by █ but keep it defined
			if bgN > 0 {
				bg = rgbColor{uint8(bgSum[0] / bgN), uint8(bgSum[1] / bgN), uint8(bgSum[2] / bgN)}
			}
			line.WriteString(styleFor(fg, bg).Render(quadrantChars[mask]))
		}
		rows = append(rows, line.String())
	}
	return strings.Join(rows, "\n")
}

// boxScale downscales img to dstW×dstH by averaging every source pixel that
// falls in each target cell (box filter) — smooth and free of the aliasing a
// single-sample (nearest-neighbor) scale produces.
func boxScale(img image.Image, dstW, dstH int) [][]rgbColor {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	out := make([][]rgbColor, dstH)
	for y := 0; y < dstH; y++ {
		out[y] = make([]rgbColor, dstW)
		sy0 := y * srcH / dstH
		sy1 := (y + 1) * srcH / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < dstW; x++ {
			sx0 := x * srcW / dstW
			sx1 := (x + 1) * srcW / dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var rSum, gSum, bSum, n uint64
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					r, g, b, _ := img.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					rSum += uint64(r >> 8)
					gSum += uint64(g >> 8)
					bSum += uint64(b >> 8)
					n++
				}
			}
			out[y][x] = rgbColor{uint8(rSum / n), uint8(gSum / n), uint8(bSum / n)}
		}
	}
	return out
}
