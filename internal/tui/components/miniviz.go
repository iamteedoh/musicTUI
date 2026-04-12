package components

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type MiniVisualizer struct {
	spectrum  *audio.SharedSpectrum
	isPlaying bool
	heights   []float64 // smoothed display heights (0-1)
	prevDots  []int     // previous frame's dot counts per bar (for hysteresis)
	tick      int
	cached    string // cached render output
	lastW     int
	lastH     int
}

func NewMiniVisualizer() MiniVisualizer {
	return MiniVisualizer{}
}

func (v *MiniVisualizer) SetSpectrum(s *audio.SharedSpectrum) { v.spectrum = s }
func (v *MiniVisualizer) SetPosition(_ int64)                 {}
func (v *MiniVisualizer) Update(playing bool) {
	v.isPlaying = playing
	v.tick++
}

func (v *MiniVisualizer) View(th theme.Theme, width, height int) string {
	numBars := width - 4
	if numBars < 4 {
		numBars = 4
	}
	if height < 2 {
		height = 2
	}

	// Only re-render every 4th tick (~15fps) to prevent flicker
	// Return cached result on intermediate frames
	sizeChanged := width != v.lastW || height != v.lastH
	if v.tick%4 != 0 && v.cached != "" && !sizeChanged {
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
		v.prevDots = make([]int, numBars)
	}

	// Read spectrum data
	if v.spectrum != nil && v.isPlaying {
		data := v.spectrum.Read()

		activeBins := 0
		var peak float64
		for i := 0; i < audio.NumBins; i++ {
			mag := float64(data.Magnitudes[i])
			if mag > 0.0001 {
				activeBins = i + 1
			}
			if mag > peak {
				peak = mag
			}
		}

		if activeBins >= 2 && peak > 0.0001 {
			for i := 0; i < numBars; i++ {
				binIdx := i * activeBins / numBars
				if binIdx >= audio.NumBins {
					binIdx = audio.NumBins - 1
				}
				mag := float64(data.Magnitudes[binIdx])
				normalized := mag / peak
				var target float64
				if normalized > 0 {
					target = math.Log10(1 + normalized*9)
				}

				// Smooth: rise fast, fall slow
				diff := target - v.heights[i]
				if diff > 0 {
					v.heights[i] += diff * 0.6
				} else {
					v.heights[i] += diff * 0.2
				}
			}
		} else if !v.isPlaying {
			for i := range v.heights {
				v.heights[i] *= 0.85
			}
		}
	} else if !v.isPlaying {
		for i := range v.heights {
			v.heights[i] *= 0.85
		}
	}

	// Render with hysteresis: a dot only changes state if height moves
	// at least 0.5 dots past the threshold (prevents boundary oscillation)
	totalDotRows := height * 4
	accent := th.Accent
	accentDim := th.AccentDim

	var rows []string
	for row := 0; row < height; row++ {
		var line strings.Builder
		line.WriteString(padStr)

		for col := 0; col < numBars; col++ {
			barDotsRaw := v.heights[col] * float64(totalDotRows)

			// Hysteresis: if previous dot count was N, only change if
			// raw value moves more than 0.6 dots away from N
			prevDots := v.prevDots[col]
			barDots := prevDots
			if barDotsRaw > float64(prevDots)+0.6 {
				barDots = int(barDotsRaw)
			} else if barDotsRaw < float64(prevDots)-0.6 {
				barDots = int(barDotsRaw)
			}
			if row == 0 { // update prevDots once per frame (first row)
				v.prevDots[col] = barDots
			}

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
			} else {
				ch := rune(0x2800 + mask)
				dotPos := float64(totalDotRows-(row*4)-1) / float64(totalDotRows)
				var color lipgloss.Color
				if dotPos > 0.6 {
					color = accent
				} else {
					color = accentDim
				}
				line.WriteString(lipgloss.NewStyle().Foreground(color).Render(string(ch)))
			}
		}
		rows = append(rows, line.String())
	}

	v.cached = strings.Join(rows, "\n")
	return v.cached
}
