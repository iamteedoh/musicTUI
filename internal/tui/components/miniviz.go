package components

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// This visualizer mirrors CAVA's audio processing (the engine behind the
// "Kurve" Plasma widget) so the on-screen motion matches the beat/tempo the
// same way. The look comes from three CAVA mechanisms — NOT spatial spreading
// (monstercat/waves are off in Kurve) — applied to the FFT magnitudes:
//
//   1. log-distributed frequency bars between a low and high cutoff,
//   2. a gravity falloff filter (snap up; fall with acceleration), and
//   3. a leaky-integral time filter,
//
// both driven by `noise_reduction`, plus auto-sensitivity that keeps the
// loudest bars near full height. Constants/formulas are ported from
// cava's cavacore.c (cava_execute).
const (
	vizFramerate   = 60.0    // matches the app tick rate; CAVA's framerate_mod keeps response real-time-correct
	vizLowCutoffHz = 20.0    // CAVA lower_cutoff_freq (Kurve: 20)
	vizHiCutoffHz  = 10000.0 // CAVA higher_cutoff_freq (Kurve: 10000)
	vizNyquistHz   = 44100.0 / 2.0
	vizFFTBins     = 1024.0 // FFT_SIZE/2 in the bridge (2048/2)

	// Kurve's CAVA smoothing config is noise_reduction=18 (i.e. 0.18 after the
	// /100 cava applies). Low value = snappy/responsive → beats pop. Override at
	// runtime with MUSICTUI_VIZ_SMOOTHING (same 0-100 scale as cava's config).
	vizNoiseReductionDefault = 18.0

	// The bridge hands us dB-compressed bins ((20*log10(mag)+60)/60). CAVA works
	// on LINEAR magnitudes, so we undo that curve before the filters — otherwise
	// every bar looks lifted/pinned (a -20 dB bar would draw at 67% height).
	// linear = 10^((dr/20)*(b-1)); dr=60 exactly inverts the bridge. Lower dr =
	// more body, higher = spikier. Override with MUSICTUI_VIZ_DYNRANGE_DB.
	vizDynRangeDBDefault = 60.0

	// Output gain applied at draw time. Auto-sensitivity parks the single
	// loudest bar near full height, so in a short TUI strip everything else sits
	// low. This lifts the peaks (tallest transients just clip at the top, the
	// mid peaks grow) without raising the floor. Override with MUSICTUI_VIZ_GAIN.
	vizPeakGainDefault = 2.0

	vizSat        = 0.85 // rainbow saturation
	vizLightBase  = 0.30 // glow brightness at the base of a bar
	vizLightRange = 0.32 // extra brightness toward the top
	vizHueSpan    = 300.0 // hue sweep across the width (red→…→magenta)
)

type MiniVisualizer struct {
	spectrum  *audio.SharedSpectrum
	isPlaying bool

	// Per-bar CAVA state (2 bars per terminal cell for horizontal resolution).
	heights []float64 // current smoothed output (0-1 at draw, clamped)
	peak    []float64 // value a falling bar decays from (cava_peak)
	fall    []float64 // gravity fall accumulator (cava_fall)
	mem     []float64 // leaky-integral memory (cava_mem)
	prevOut []float64 // previous frame's output, pre-integral (prev_cava_out)
	srcLo   []int     // log-freq → source-bin mapping (low index)
	srcFrac []float64 // … and interpolation fraction

	sens     float64 // auto-sensitivity multiplier (cava sens)
	sensInit bool    // fast initial gain ramp (cava sens_init)

	// Precomputed CAVA smoothing coefficients (depend on framerate + noise_reduction).
	noiseReduction float64
	gravityMod     float64
	integralCoef   float64
	framerateMod   float64
	dynRangeCoef   float64 // dr/20 — undoes the bridge's dB compression (see above)
	peakGain       float64 // draw-time amplitude lift

	// Precomputed per-cell ANSI color prefixes (hue by column, glow by row) plus
	// the shared reset suffix. Built once per resize so the hot render loop is
	// just string concatenation — no per-cell lipgloss/termenv work each frame.
	prefixTable [][]string
	resetSuffix string

	bpm    int
	tick   int
	cached string
	lastW  int
	lastH  int
}

// NewMiniVisualizer returns a pointer because the App holds it across
// value-receiver View() calls — its per-frame state (smoothing, precomputed
// colors, resize cache) MUST persist between frames, which a value field copied
// each frame would silently discard (forcing a full rebuild every frame).
func NewMiniVisualizer() *MiniVisualizer {
	// noise_reduction (0-100 like cava's config), overridable at runtime.
	nr := vizNoiseReductionDefault
	if s := os.Getenv("MUSICTUI_VIZ_SMOOTHING"); s != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && f > 0 {
			nr = f
		}
	}
	nr /= 100.0 // cava normalizes config 0-100 → 0-1

	dr := vizDynRangeDBDefault
	if s := os.Getenv("MUSICTUI_VIZ_DYNRANGE_DB"); s != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && f > 0 {
			dr = f
		}
	}

	gain := vizPeakGainDefault
	if s := os.Getenv("MUSICTUI_VIZ_GAIN"); s != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && f > 0 {
			gain = f
		}
	}

	framerateMod := 66.0 / vizFramerate
	return &MiniVisualizer{
		sens:           1.0,
		sensInit:       true,
		noiseReduction: nr,
		framerateMod:   framerateMod,
		gravityMod:     math.Pow(framerateMod, 2.5) * 2.0 / nr,
		integralCoef:   nr / math.Pow(framerateMod, 0.1),
		dynRangeCoef:   dr / 20.0,
		peakGain:       gain,
	}
}

func (v *MiniVisualizer) SetSpectrum(s *audio.SharedSpectrum) { v.spectrum = s }
func (v *MiniVisualizer) SetPosition(_ int64)                 {}
func (v *MiniVisualizer) Update(playing bool)                 { v.isPlaying = playing; v.tick++ }

// LastBPM returns the most recent tempo estimate (0 if unknown).
func (v *MiniVisualizer) LastBPM() int { return v.bpm }

// Braille dot bits, top→bottom, for the left and right sub-columns of a cell —
// lets one terminal cell render two bars, doubling horizontal resolution.
var (
	leftDotBits  = [4]int{0x01, 0x02, 0x04, 0x40}
	rightDotBits = [4]int{0x08, 0x10, 0x20, 0x80}
)

func (v *MiniVisualizer) View(th theme.Theme, width, height int) string {
	_ = th // visualizer uses its own rainbow palette (CAVA/Kurve look)

	cells := width - 4
	if cells < 4 {
		cells = 4
	}
	if height < 2 {
		height = 2
	}
	numBars := cells * 2 // two bars per terminal cell

	// Rebuild size-dependent tables on resize.
	if width != v.lastW || height != v.lastH || len(v.heights) != numBars {
		v.lastW = width
		v.lastH = height
		v.rebuild(cells, height, numBars)
	}

	// ── Advance CAVA's per-bar smoothing from the latest spectrum frame ──
	if v.spectrum != nil && v.isPlaying {
		v.step(v.spectrum.Read(), numBars)
	} else if !v.isPlaying {
		for i := range v.heights {
			v.heights[i] *= 0.85
			v.mem[i] *= 0.85
			v.prevOut[i] *= 0.85
			v.peak[i] *= 0.85
		}
	}

	// ── Render Braille curve with the precomputed rainbow palette ──
	totalDotRows := height * 4
	var rows []string
	for row := 0; row < height; row++ {
		var line strings.Builder
		line.WriteString(strings.Repeat(" ", (width-cells)/2))
		for c := 0; c < cells; c++ {
			leftDots := int(clamp01(v.heights[2*c]*v.peakGain) * float64(totalDotRows))
			rightDots := int(clamp01(v.heights[2*c+1]*v.peakGain) * float64(totalDotRows))

			mask := 0
			for dy := 0; dy < 4; dy++ {
				dotFromBottom := totalDotRows - (row*4 + dy) - 1
				if dotFromBottom < leftDots {
					mask |= leftDotBits[dy]
				}
				if dotFromBottom < rightDots {
					mask |= rightDotBits[dy]
				}
			}
			if mask == 0 {
				line.WriteByte(' ')
				continue
			}
			line.WriteString(v.prefixTable[c][row])
			line.WriteRune(rune(0x2800 + mask))
		}
		line.WriteString(v.resetSuffix) // restore default color at line end
		rows = append(rows, line.String())
	}

	v.cached = strings.Join(rows, "\n")
	return v.cached
}

// step runs one frame of CAVA's gravity + integral + autosens filters.
func (v *MiniVisualizer) step(data audio.SpectrumData, numBars int) {
	if data.BPM > 0 {
		v.bpm = int(data.BPM + 0.5)
	}

	var maxRaw, maxOut float64
	for n := 0; n < numBars; n++ {
		// Sample the bridge bins at this bar's log-frequency position, then undo
		// the bridge's dB compression so CAVA's filters see LINEAR amplitude
		// (low floor, sharp peaks) instead of a lifted/pinned dome.
		lo := v.srcLo[n]
		b := float64(data.Magnitudes[lo])*(1-v.srcFrac[n]) + float64(data.Magnitudes[lo+1])*v.srcFrac[n]
		var rawF float64
		if b > 0 {
			rawF = math.Pow(10, v.dynRangeCoef*(b-1))
		}
		if rawF > maxRaw {
			maxRaw = rawF
		}

		val := rawF * v.sens

		// Gravity falloff (cava_execute): when the new value is below the last,
		// fall from the held peak with accelerating speed.
		if val < v.prevOut[n] && v.noiseReduction > 0.1 {
			val = v.peak[n] * (1.0 - v.fall[n]*v.fall[n]*v.gravityMod)
			if val < 0 {
				val = 0
			}
			v.fall[n] += 0.028
		} else {
			v.peak[n] = val
			v.fall[n] = 0
		}
		v.prevOut[n] = val

		// Leaky-integral time smoothing.
		val = v.mem[n]*v.integralCoef + val
		v.mem[n] = val

		v.heights[n] = val
		if val > maxOut {
			maxOut = val
		}
	}

	// Auto-sensitivity (cava): back off hard on overshoot, creep up otherwise so
	// the loudest bars settle near full height regardless of track loudness.
	if maxOut > 1.0 {
		v.sens *= 1.0 - 0.02*v.framerateMod
		v.sensInit = false
	} else if maxRaw > 0.005 { // not silence
		v.sens *= 1.0 + 0.001*v.framerateMod
		if v.sensInit {
			v.sens *= 1.0 + 0.1*v.framerateMod
		}
	}
	if v.sens < 1e-4 {
		v.sens = 1e-4
	} else if v.sens > 1e6 {
		v.sens = 1e6
	}
}

// rebuild recomputes the log-frequency bin mapping and the per-cell color table.
func (v *MiniVisualizer) rebuild(cells, height, numBars int) {
	v.heights = make([]float64, numBars)
	v.peak = make([]float64, numBars)
	v.fall = make([]float64, numBars)
	v.mem = make([]float64, numBars)
	v.prevOut = make([]float64, numBars)
	v.srcLo = make([]int, numBars)
	v.srcFrac = make([]float64, numBars)

	// Log frequency distribution between the cutoffs (CAVA-style), mapped back
	// to the bridge's source bins. The bridge spaces its NumBins quadratically
	// in frequency: source bin b ≈ NumBins*sqrt(f / Nyquist) (since FFT bin =
	// f/binWidth and source bin = NumBins*sqrt(FFTbin/halfSize)).
	denom := 1.0
	if numBars > 1 {
		denom = float64(numBars - 1)
	}
	ratio := vizHiCutoffHz / vizLowCutoffHz
	for n := 0; n < numBars; n++ {
		f := vizLowCutoffHz * math.Pow(ratio, float64(n)/denom)
		src := float64(audio.NumBins) * math.Sqrt(f/vizNyquistHz)
		if src < 0 {
			src = 0
		}
		if src > float64(audio.NumBins-2) {
			src = float64(audio.NumBins - 2) // leave room for the +1 interpolation
		}
		lo := int(src)
		v.srcLo[n] = lo
		v.srcFrac[n] = src - float64(lo)
	}

	// Per-cell palette: hue by column (rainbow), lightness/glow by row. The
	// color escape for each cell is computed once here (via lipgloss, so it
	// honors the terminal's color profile / degrades on non-truecolor terms) and
	// reused every frame — re-styling per cell was the main render cost.
	const sentinel = "M" // never appears inside an SGR sequence (which ends in 'm')
	v.prefixTable = make([][]string, cells)
	v.resetSuffix = ""
	for c := 0; c < cells; c++ {
		hue := (float64(c) + 0.5) / float64(cells) * vizHueSpan
		v.prefixTable[c] = make([]string, height)
		for row := 0; row < height; row++ {
			vpos := (float64(height-row) - 0.5) / float64(height) // 0 bottom .. 1 top
			light := vizLightBase + vpos*vizLightRange
			sample := lipgloss.NewStyle().
				Foreground(lipgloss.Color(hslHex(hue, vizSat, light))).
				Render(sentinel)
			if i := strings.Index(sample, sentinel); i >= 0 {
				v.prefixTable[c][row] = sample[:i]
				v.resetSuffix = sample[i+len(sentinel):]
			}
		}
	}
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// hslHex converts an HSL color to "#RRGGBB". h in [0,360), s and l in [0,1].
func hslHex(h, s, l float64) string {
	cc := (1 - math.Abs(2*l-1)) * s
	hp := math.Mod(h, 360) / 60.0
	x := cc * (1 - math.Abs(math.Mod(hp, 2)-1))
	var r, g, b float64
	switch {
	case hp < 1:
		r, g, b = cc, x, 0
	case hp < 2:
		r, g, b = x, cc, 0
	case hp < 3:
		r, g, b = 0, cc, x
	case hp < 4:
		r, g, b = 0, x, cc
	case hp < 5:
		r, g, b = x, 0, cc
	default:
		r, g, b = cc, 0, x
	}
	m := l - cc/2
	return fmt.Sprintf("#%02X%02X%02X",
		clamp8((r+m)*255), clamp8((g+m)*255), clamp8((b+m)*255))
}

func clamp8(f float64) int {
	if f < 0 {
		return 0
	}
	if f > 255 {
		return 255
	}
	return int(f)
}
