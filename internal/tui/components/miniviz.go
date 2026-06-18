package components

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// Tuning constants for the visualizer's motion and beat response.
const (
	vizAttack     = 0.55  // how fast bars rise toward a louder target
	vizDecay      = 0.16  // how fast bars fall (slower than rise feels natural)
	vizAGCDecay   = 0.995 // slow gain tracking so loud/quiet sections differ
	vizAGCFloor   = 0.18  // minimum gain reference (quiet passages don't blow up)
	vizPulseDecay = 0.85  // beat-pulse fade per render frame
	vizBeatBoost  = 0.22  // extra global height on a beat (the "pump")
	vizUsefulBins = 112   // ignore the top ~mostly-empty bins for a fuller look
)

type MiniVisualizer struct {
	spectrum  *audio.SharedSpectrum
	isPlaying bool
	heights   []float64 // smoothed per-bar heights (0-1)
	agc       float64   // slow-tracking gain reference (recent loudness ceiling)
	pulse     float64   // beat-pulse envelope (0-1)
	bpm       int       // last estimated tempo, for display
	tick      int
	cached    string
	lastW     int
	lastH     int
}

func NewMiniVisualizer() MiniVisualizer {
	return MiniVisualizer{agc: vizAGCFloor}
}

func (v *MiniVisualizer) SetSpectrum(s *audio.SharedSpectrum) { v.spectrum = s }
func (v *MiniVisualizer) SetPosition(_ int64)                 {}
func (v *MiniVisualizer) Update(playing bool)                 { v.isPlaying = playing; v.tick++ }

// LastBPM returns the most recent tempo estimate (0 if unknown).
func (v *MiniVisualizer) LastBPM() int { return v.bpm }

func (v *MiniVisualizer) View(th theme.Theme, width, height int) string {
	numBars := width - 4
	if numBars < 4 {
		numBars = 4
	}
	if height < 2 {
		height = 2
	}

	// Render at ~30fps (every 2nd tick of the 60fps clock). The spectrum now
	// arrives at ~60Hz, so this is smooth while halving render cost.
	sizeChanged := width != v.lastW || height != v.lastH
	if v.tick%2 != 0 && v.cached != "" && !sizeChanged {
		return v.cached
	}
	v.lastW = width
	v.lastH = height

	leftPad := (width - numBars) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	if len(v.heights) != numBars {
		v.heights = make([]float64, numBars)
	}

	// ── Advance the simulation from the latest spectrum frame ──
	if v.spectrum != nil && v.isPlaying {
		data := v.spectrum.Read()

		// Slow AGC: track a decaying loudness ceiling so beats "pump" instead
		// of being flattened by per-frame normalization.
		var frameMax float64
		for i := 0; i < audio.NumBins && i < vizUsefulBins; i++ {
			if m := float64(data.Magnitudes[i]); m > frameMax {
				frameMax = m
			}
		}
		v.agc = math.Max(frameMax, v.agc*vizAGCDecay)
		if v.agc < vizAGCFloor {
			v.agc = vizAGCFloor
		}
		gain := 1.0 / v.agc

		// Beat pulse: latch the rise, decay smoothly at render rate.
		if bi := float64(data.BeatIntensity); bi > v.pulse {
			v.pulse = bi
		} else {
			v.pulse *= vizPulseDecay
		}
		if data.BPM > 0 {
			v.bpm = int(data.BPM + 0.5)
		}

		for i := 0; i < numBars; i++ {
			// Each bar spans a contiguous group of bins; take the max to keep
			// transient peaks crisp.
			start := i * vizUsefulBins / numBars
			end := (i + 1) * vizUsefulBins / numBars
			if end <= start {
				end = start + 1
			}
			var barMax float64
			for b := start; b < end && b < audio.NumBins; b++ {
				if m := float64(data.Magnitudes[b]); m > barMax {
					barMax = m
				}
			}
			target := barMax * gain
			if target > 1 {
				target = 1
			}

			// Asymmetric envelope: rise fast, fall slower.
			diff := target - v.heights[i]
			if diff > 0 {
				v.heights[i] += diff * vizAttack
			} else {
				v.heights[i] += diff * vizDecay
			}
		}
	} else if !v.isPlaying {
		for i := range v.heights {
			v.heights[i] *= 0.85
		}
		if v.pulse *= vizPulseDecay; v.pulse < 0.001 {
			v.pulse = 0
		}
	}

	// Global beat boost applied at draw time (doesn't accumulate in heights).
	boost := 1.0 + v.pulse*vizBeatBoost

	// ── Render Braille bars with a vertical color gradient + beat flash ──
	totalDotRows := height * 4
	dimHex := colorHex(th.AccentDim, "#4C566A")
	accentHex := colorHex(th.Accent, "#88C0D0")
	hotHex := colorHex(th.Warning, "#EBCB8B")

	var rows []string
	for row := 0; row < height; row++ {
		var line strings.Builder
		line.WriteString(padStr)
		for col := 0; col < numBars; col++ {
			h := v.heights[col] * boost
			if h > 1 {
				h = 1
			}
			barDots := int(h * float64(totalDotRows))

			var mask int
			for dy := 0; dy < 4; dy++ {
				dotFromBottom := totalDotRows - (row*4 + dy) - 1
				if dotFromBottom < barDots {
					switch dy {
					case 0:
						mask |= 1 << 0
					case 1:
						mask |= 1 << 1
					case 2:
						mask |= 1 << 2
					case 3:
						mask |= 1 << 6
					}
				}
			}

			if mask == 0 {
				line.WriteString(" ")
				continue
			}
			ch := rune(0x2800 + mask)
			// Cell's vertical position (0 bottom .. 1 top).
			dotPos := float64(totalDotRows-(row*4)) / float64(totalDotRows)
			// Gradient dim→accent by height, then flash toward hot on the beat.
			c := lerpHex(dimHex, accentHex, dotPos)
			if v.pulse > 0.01 && dotPos > 0.45 {
				c = lerpHex(c, hotHex, v.pulse*(dotPos-0.45)/0.55)
			}
			line.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(string(ch)))
		}
		rows = append(rows, line.String())
	}

	v.cached = strings.Join(rows, "\n")
	return v.cached
}

// colorHex returns a theme color as a "#RRGGBB" string, falling back to def if
// it isn't a parseable 7-char hex color (e.g. a named/ANSI color).
func colorHex(c lipgloss.Color, def string) string {
	s := string(c)
	if len(s) == 7 && s[0] == '#' {
		return s
	}
	return def
}

// lerpHex linearly interpolates between two "#RRGGBB" colors (t in 0..1).
func lerpHex(a, b string, t float64) string {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	ar, ag, ab := hexRGB(a)
	br, bg, bb := hexRGB(b)
	r := int(float64(ar) + (float64(br)-float64(ar))*t)
	g := int(float64(ag) + (float64(bg)-float64(ag))*t)
	bl := int(float64(ab) + (float64(bb)-float64(ab))*t)
	return fmt.Sprintf("#%02X%02X%02X", r, g, bl)
}

func hexRGB(s string) (int, int, int) {
	if len(s) != 7 || s[0] != '#' {
		return 255, 255, 255
	}
	r, _ := strconv.ParseInt(s[1:3], 16, 0)
	g, _ := strconv.ParseInt(s[3:5], 16, 0)
	b, _ := strconv.ParseInt(s[5:7], 16, 0)
	return int(r), int(g), int(b)
}
