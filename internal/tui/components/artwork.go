package components

import (
	"fmt"
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
	a.mu.RLock()
	defer a.mu.RUnlock()

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

	imgStr := a.renderHalfBlocks(width, imgHeight)

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

// renderHalfBlocks draws the artwork as solid color using the upper-half
// block (▀): each character cell shows TWO image pixels — foreground colors
// the top half, background colors the bottom half. Unlike the previous
// braille dot-matrix (sparse dots, dark areas rendered as blank space, one
// color per 8 dots), every cell is fully painted in true color, so the art
// reads as a continuous image. Downscaling box-filters the source (averages
// every source pixel in the target region) to avoid sampling noise.
func (a *Artwork) renderHalfBlocks(width, height int) string {
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Terminal cells are ~1:2 (w:h); one cell = 1 pixel wide × 2 pixels tall,
	// which makes the sub-pixels roughly square, so a square cover stays square.
	charW := width - 2
	charH := height
	pxW := charW
	pxH := charH * 2

	// Fit preserving aspect ratio.
	scale := float64(srcW) / float64(pxW)
	if s := float64(srcH) / float64(pxH); s > scale {
		scale = s
	}
	actualPxW := int(float64(srcW) / scale)
	actualPxH := int(float64(srcH) / scale)
	if actualPxW < 1 {
		actualPxW = 1
	}
	if actualPxH < 2 {
		actualPxH = 2
	}
	actualCharW := actualPxW
	actualCharH := actualPxH / 2

	// Box-filter the source into the target pixel grid.
	px := boxScale(a.img, actualPxW, actualCharH*2)

	leftPad := (width - actualCharW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - actualCharH) / 2
	if topPad < 0 {
		topPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	// Styles repeat heavily across cells (flat color areas); cache them.
	styles := make(map[uint64]lipgloss.Style)
	styleFor := func(top, bot rgbColor) lipgloss.Style {
		key := uint64(top.R)<<40 | uint64(top.G)<<32 | uint64(top.B)<<24 |
			uint64(bot.R)<<16 | uint64(bot.G)<<8 | uint64(bot.B)
		st, ok := styles[key]
		if !ok {
			st = lipgloss.NewStyle().
				Foreground(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", top.R, top.G, top.B))).
				Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", bot.R, bot.G, bot.B)))
			styles[key] = st
		}
		return st
	}

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	for cy := 0; cy < actualCharH; cy++ {
		var line strings.Builder
		line.WriteString(padStr)
		for cx := 0; cx < actualCharW; cx++ {
			line.WriteString(styleFor(px[cy*2][cx], px[cy*2+1][cx]).Render("▀"))
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
