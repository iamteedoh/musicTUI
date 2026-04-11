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
	"github.com/iamteedoh/musictui-go/internal/theme"
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
func (a *Artwork) SetTrackInfo(_, _, _ string)                  {}

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

	imgStr := a.renderDotMatrix(width, imgHeight)

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

// Bayer 4x2 ordered dither matrix
var bayer = [4][2]float64{
	{0.1, 0.6},
	{0.8, 0.3},
	{0.2, 0.7},
	{0.9, 0.4},
}

func (a *Artwork) renderDotMatrix(width, height int) string {
	bounds := a.img.Bounds()
	srcW := float64(bounds.Max.X - bounds.Min.X)
	srcH := float64(bounds.Max.Y - bounds.Min.Y)

	charW := width - 2
	charH := height
	dotW := charW * 2
	dotH := charH * 4

	scaleX := srcW / float64(dotW)
	scaleY := srcH / float64(dotH)
	scale := scaleX
	if scaleY > scale {
		scale = scaleY
	}
	if scale < 1 {
		scale = 1
	}

	actualCharW := int(srcW / scale / 2)
	actualCharH := int(srcH / scale / 4)

	leftPad := (width - actualCharW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - actualCharH) / 2
	if topPad < 0 {
		topPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}

	for cy := 0; cy < actualCharH; cy++ {
		var line strings.Builder
		line.WriteString(padStr)

		for cx := 0; cx < actualCharW; cx++ {
			var rSum, gSum, bSum float64
			var count float64
			var mask int

			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					sx := int(float64(cx*2+dx) * scale)
					sy := int(float64(cy*4+dy) * scale)

					if sx >= int(srcW) {
						sx = int(srcW) - 1
					}
					if sy >= int(srcH) {
						sy = int(srcH) - 1
					}

					r, g, b, _ := a.img.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)
					lum := (0.299*rf + 0.587*gf + 0.114*bf) / 255.0

					rSum += rf
					gSum += gf
					bSum += bf
					count++

					// Dither: turn on dot if luminance is above bayer threshold
					if lum > bayer[dy][dx] {
						var bit int
						switch {
						case dx == 0 && dy < 3:
							bit = dy
						case dx == 1 && dy < 3:
							bit = dy + 3
						case dx == 0 && dy == 3:
							bit = 6
						case dx == 1 && dy == 3:
							bit = 7
						}
						mask |= 1 << bit
					}
				}
			}

			if mask == 0 {
				line.WriteString(" ")
				continue
			}

			// Use average color for the active dots
			ru, gu, bu := boostColor(uint8(rSum/count), uint8(gSum/count), uint8(bSum/count))
			col := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", ru, gu, bu))

			char := rune(0x2800 + mask)
			line.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(char)))
		}
		rows = append(rows, line.String())
	}

	return strings.Join(rows, "\n")
}

// boostColor increases saturation and brightness for better visibility in TUI.
func boostColor(r, g, b uint8) (uint8, uint8, uint8) {
	rf, gf, bf := float64(r), float64(g), float64(b)
	avg := (rf + gf + bf) / 3.0

	// Boost saturation 1.2x
	rf = avg + (rf-avg)*1.2
	gf = avg + (gf-avg)*1.2
	bf = avg + (bf-avg)*1.2

	// Brightness lift 1.1x
	rf *= 1.1
	gf *= 1.1
	bf *= 1.1

	if rf > 255 { rf = 255 }
	if gf > 255 { gf = 255 }
	if bf > 255 { bf = 255 }
	if rf < 0 { rf = 0 }
	if gf < 0 { gf = 0 }
	if bf < 0 { bf = 0 }

	return uint8(rf), uint8(gf), uint8(bf)
}
