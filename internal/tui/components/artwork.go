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

	// style selects the renderer (blocks / braille / kitty pixels).
	//
	// Kitty-graphics state: in StyleKitty the panel renders a Unicode-
	// placeholder grid and queues the protocol escapes (transmit /
	// placement / delete) in oob; the app layer flushes that queue straight
	// to the terminal between frames (it must bypass Bubble Tea's diffing).
	style          ArtworkStyle
	kittyID        uint32
	transmittedURL string // image URL already transmitted under kittyID
	placedCols     int
	placedRows     int
	oob            strings.Builder

	// Sixel state. Unlike kitty, a sixel image is painted at the cursor with
	// nothing binding it to the frame, so it needs the panel's absolute screen
	// position and the terminal's pixel-per-cell size. Zero values mean "we
	// weren't told", and the renderer falls back to blocks rather than guess.
	cellW, cellH int
	originCol    int // 1-based column of the artwork content area
	originRow    int // 1-based row of the artwork content area
	sixelURL     string
	sixelCols    int
	sixelRows    int
	sixelPayload string // encoded DCS for (sixelURL, sixelCols, sixelRows)

	// The cursor-positioned payload and the screen rows it covers. The app
	// redraws it whenever Bubble Tea rewrites any of those rows — which it does
	// whole-line, so a change in the left or center column erases the pixels
	// sharing that line.
	drawSeq  string
	drawRow  int
	drawRows int
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

// SetStyle selects the artwork renderer. Call once at startup after
// terminal detection (see DetectArtworkStyle).
func (a *Artwork) SetStyle(s ArtworkStyle) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.style = s
}

// SetCellSize records the terminal's pixel-per-cell size, as reported by the
// probe. Required by the sixel renderer to scale the cover onto whole cells.
func (a *Artwork) SetCellSize(w, h int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cellW, a.cellH = w, h
}

// SetOrigin records where the artwork content area starts on screen, as a
// 1-based (column, row). The sixel payload is cursor-positioned there. Call
// from the layout, which is the only place that knows.
//
// No invalidation is needed on a move: the payload is re-queued on every
// panel render, always against the current origin.
func (a *Artwork) SetOrigin(col, row int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.originCol, a.originRow = col, row
}

// Origin returns the 1-based (column, row) last given to SetOrigin, so the
// layout's hand-derived coordinates can be checked against a rendered frame.
func (a *Artwork) Origin() (col, row int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.originCol, a.originRow
}

// SixelDraw returns the cursor-positioned payload for the current cover and the
// 1-based screen rows it covers. Empty when nothing is drawn with sixel.
//
// Bubble Tea's line diff rewrites a whole line whenever any column on it
// changes, which erases pixels sharing that line — so the app repaints the
// image whenever one of these rows is rewritten.
func (a *Artwork) SixelDraw() (seq string, row, rows int) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.drawSeq, a.drawRow, a.drawRows
}

// clearSixelDraw forgets the placed image. Caller must hold a.mu.
func (a *Artwork) clearSixelDraw() {
	a.drawSeq, a.drawRow, a.drawRows = "", 0, 0
}

// TakeOOB drains the queued graphics escapes — kitty protocol payloads or a
// cursor-positioned sixel image. The app layer writes them directly to the
// terminal (out of band of Bubble Tea's renderer): they carry image data, not
// visible text, so they must not be diffed, cached, or truncated like view
// content.
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

	// No cover on screen: drop any placed image so the app stops repainting it
	// over the placeholder.
	if a.loading || a.err != "" || a.img == nil {
		a.clearSixelDraw()
	}
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
	switch a.style {
	case StyleKitty:
		a.clearSixelDraw()
		imgStr = a.renderPlaceholders(width, imgHeight)
	case StyleSixel:
		imgStr = a.renderSixel(width, imgHeight)
	case StyleBraille:
		a.clearSixelDraw()
		imgStr = a.renderBraille(width, imgHeight)
	default:
		a.clearSixelDraw()
		imgStr = a.renderBlocks(width, imgHeight)
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
			return a.renderBlocks(width, height)
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

// renderSixel paints the cover as real pixels via a sixel DCS payload, and
// returns the blank cells the image sits on. Caller must hold a.mu.
//
// The payload is memoized on (image, grid size) because encoding a cover costs
// milliseconds, but it is re-queued on EVERY call. A call means the artwork
// panel is being re-rendered, which means Bubble Tea is about to repaint those
// cells and wipe the pixels — so the image has to be drawn again behind it. On
// a view-cache hit this function never runs and the pixels simply persist.
//
// Falls back to blocks whenever we lack something we'd otherwise have to guess:
// the terminal's cell size, or the panel's screen position.
func (a *Artwork) renderSixel(width, height int) string {
	a.clearSixelDraw() // republished below; every early return leaves it cleared
	if a.cellW <= 0 || a.cellH <= 0 || a.originCol <= 0 || a.originRow <= 0 {
		return a.renderBlocks(width, height)
	}
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}
	charW := width - 2
	charH := height
	if charW < 1 || charH < 1 {
		return a.renderBlocks(width, height)
	}

	// Fit the cover inside the panel's true pixel box, then snap down to whole
	// cells so the image can never bleed into a neighbouring cell.
	boxW := charW * a.cellW
	boxH := charH * a.cellH
	scale := float64(srcW) / float64(boxW)
	if s := float64(srcH) / float64(boxH); s > scale {
		scale = s
	}
	cellsW := int(float64(srcW)/scale) / a.cellW
	cellsH := int(float64(srcH)/scale) / a.cellH
	if cellsW < 1 {
		cellsW = 1
	}
	if cellsH < 1 {
		cellsH = 1
	}
	pxW := cellsW * a.cellW
	pxH := cellsH * a.cellH

	leftPad := (width - cellsW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := (height - cellsH) / 2
	if topPad < 0 {
		topPad = 0
	}

	if a.sixelPayload == "" || a.sixelURL != a.imageURL ||
		a.sixelCols != cellsW || a.sixelRows != cellsH {
		payload, err := sixelEncode(a.img, pxW, pxH)
		if err != nil {
			return a.renderBlocks(width, height)
		}
		a.sixelPayload = payload
		a.sixelURL = a.imageURL
		a.sixelCols, a.sixelRows = cellsW, cellsH
	}
	// Publish the draw for the app layer, which decides when the pixels need
	// repainting. Writing it here would be one frame too early anyway.
	a.drawSeq = sixelAt(a.originRow+topPad, a.originCol+leftPad, a.sixelPayload)
	a.drawRow = a.originRow + topPad
	a.drawRows = cellsH

	// Blank cells beneath the pixels. Identical every frame, so once painted
	// Bubble Tea's line diff leaves them — and the image — alone.
	rows := make([]string, 0, topPad+cellsH)
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	blank := strings.Repeat(" ", leftPad+cellsW)
	for r := 0; r < cellsH; r++ {
		rows = append(rows, blank)
	}
	return strings.Join(rows, "\n")
}

// renderBraille draws the artwork as colored braille over a painted
// background: each character cell carries a 2×4 grid of image pixels (the
// same spatial detail as the original dot-matrix renderer). The eight
// subpixels are split into bright/dark clusters by luminance; the braille
// glyph paints the bright cluster in the foreground color while the
// BACKGROUND color carries the dark cluster — so, unlike the original
// braille renderer, dark regions are solid color rather than holes, and
// each cell shows two true colors instead of one. Near-uniform cells render
// as a full block (█) to avoid dot texture in flat areas. Downscaling
// box-filters the source to avoid sampling noise.
func (a *Artwork) renderBraille(width, height int) string {
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Terminal cells are ~1:2 (w:h). In "units" where a cell is 1 wide ×
	// 2 tall, braille subpixels (2×4 per cell) are 0.5 × 0.5 units — square,
	// so the cover keeps its aspect ratio.
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

	// 2×4 subpixels per cell.
	px := boxScale(a.img, cellsW*2, cellsH*4)

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

	// Braille dot bit layout: dots 1-3 = left column rows 0-2 (bits 0-2),
	// dots 4-6 = right column rows 0-2 (bits 3-5), dot 7 = left row 3
	// (bit 6), dot 8 = right row 3 (bit 7).
	brailleBit := func(dx, dy int) int {
		switch {
		case dx == 0 && dy < 3:
			return dy
		case dx == 1 && dy < 3:
			return dy + 3
		case dx == 0:
			return 6
		default:
			return 7
		}
	}

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	for cy := 0; cy < cellsH; cy++ {
		var line strings.Builder
		line.WriteString(padStr)
		for cx := 0; cx < cellsW; cx++ {
			// Cell stats first: mean/min/max luminance and the overall average
			// color, so the flat-cell path never touches cluster division.
			var mean float64
			var minL, maxL float64 = 256, -1
			var allSum [3]uint64
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					c := px[cy*4+dy][cx*2+dx]
					l := lum(c)
					mean += l
					if l < minL {
						minL = l
					}
					if l > maxL {
						maxL = l
					}
					allSum[0] += uint64(c.R)
					allSum[1] += uint64(c.G)
					allSum[2] += uint64(c.B)
				}
			}
			mean /= 8
			avg := rgbColor{uint8(allSum[0] / 8), uint8(allSum[1] / 8), uint8(allSum[2] / 8)}

			// Near-uniform cell: a solid block in the average color reads
			// cleaner than dots — and skips clustering entirely.
			if maxL-minL < 10 {
				line.WriteString(styleFor(avg, avg).Render("█"))
				continue
			}

			var mask int
			var fgSum, bgSum [3]uint64
			var fgN, bgN uint64
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					c := px[cy*4+dy][cx*2+dx]
					if lum(c) >= mean {
						mask |= 1 << brailleBit(dx, dy)
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
			}
			// Floating-point guard: with near-equal values the mean can land
			// a hair above every sample (1 ULP), leaving a cluster empty —
			// this was an integer divide-by-zero panic on resize. Render the
			// cell solid instead.
			if fgN == 0 || bgN == 0 {
				line.WriteString(styleFor(avg, avg).Render("█"))
				continue
			}

			fg := rgbColor{uint8(fgSum[0] / fgN), uint8(fgSum[1] / fgN), uint8(fgSum[2] / fgN)}
			bg := rgbColor{uint8(bgSum[0] / bgN), uint8(bgSum[1] / bgN), uint8(bgSum[2] / bgN)}
			line.WriteString(styleFor(fg, bg).Render(string(rune(0x2800 + mask))))
		}
		rows = append(rows, line.String())
	}
	return strings.Join(rows, "\n")
}

// quadrantChars maps a 4-bit subpixel mask (TL=8, TR=4, BL=2, BR=1) to the
// block element whose painted quadrants match the set bits.
var quadrantChars = [16]string{
	" ", "▗", "▖", "▄", "▝", "▐", "▞", "▟",
	"▘", "▚", "▌", "▙", "▀", "▜", "▛", "█",
}

// renderBlocks draws the artwork with quadrant block elements, choosing each
// cell's glyph by error minimization (chafa-style): every fg/bg partition of
// the cell's 2×2 subpixels is scored by squared color error against the two
// cluster averages, and the best-fitting partition wins. Flat cells render
// as a solid block. This is the default fallback for terminals without
// kitty-graphics support: photographs read smoother than dot-based braille.
func (a *Artwork) renderBlocks(width, height int) string {
	bounds := a.img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return ""
	}

	// Cells are 1×2 units; quadrant subpixels are 0.5×1 units (1:2 regions
	// of the source, which the box filter absorbs).
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

	dist2 := func(a, b rgbColor) float64 {
		dr := float64(a.R) - float64(b.R)
		dg := float64(a.G) - float64(b.G)
		db := float64(a.B) - float64(b.B)
		return dr*dr + dg*dg + db*db
	}

	var rows []string
	for i := 0; i < topPad; i++ {
		rows = append(rows, "")
	}
	for cy := 0; cy < cellsH; cy++ {
		var line strings.Builder
		line.WriteString(padStr)
		for cx := 0; cx < cellsW; cx++ {
			// Subpixels in mask-bit order: TL(8), TR(4), BL(2), BR(1).
			quad := [4]rgbColor{
				px[cy*2][cx*2], px[cy*2][cx*2+1],
				px[cy*2+1][cx*2], px[cy*2+1][cx*2+1],
			}

			bestMask, bestErr := 15, -1.0
			var bestFg, bestBg rgbColor
			// Masks 8..15 cover every partition once (lower masks are the
			// same split with fg/bg swapped, i.e. the complementary glyph).
			// Iterate from 15 down so error ties — flat cells tie at zero
			// across all partitions — resolve to the solid block.
			for mask := 15; mask >= 8; mask-- {
				var fgSum, bgSum [3]float64
				var fgN, bgN float64
				for i, c := range quad {
					if mask&(8>>i) != 0 {
						fgSum[0] += float64(c.R)
						fgSum[1] += float64(c.G)
						fgSum[2] += float64(c.B)
						fgN++
					} else {
						bgSum[0] += float64(c.R)
						bgSum[1] += float64(c.G)
						bgSum[2] += float64(c.B)
						bgN++
					}
				}
				fg := rgbColor{uint8(fgSum[0] / fgN), uint8(fgSum[1] / fgN), uint8(fgSum[2] / fgN)}
				bg := fg
				if bgN > 0 {
					bg = rgbColor{uint8(bgSum[0] / bgN), uint8(bgSum[1] / bgN), uint8(bgSum[2] / bgN)}
				}
				var err float64
				for i, c := range quad {
					if mask&(8>>i) != 0 {
						err += dist2(c, fg)
					} else {
						err += dist2(c, bg)
					}
				}
				if bestErr < 0 || err < bestErr {
					bestErr = err
					bestMask = mask
					bestFg, bestBg = fg, bg
				}
			}
			line.WriteString(styleFor(bestFg, bestBg).Render(quadrantChars[bestMask]))
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
