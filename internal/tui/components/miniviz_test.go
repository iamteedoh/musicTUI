package components

import (
	"strings"
	"testing"
	"time"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

// fakeClock advances a MiniVisualizer's notion of time by a fixed step per
// frame, so the dt-driven physics is deterministic in tests.
func fakeClock(step time.Duration) func() time.Time {
	now := time.Unix(1000, 0)
	return func() time.Time {
		now = now.Add(step)
		return now
	}
}

// The rendered frame must always be exactly the requested number of rows —
// the panel layout stacks sections by line count, so an off-by-one here
// shifts every section below the visualizer.
func TestMiniVizRendersRequestedHeight(t *testing.T) {
	th := theme.FromName("")
	v := NewMiniVisualizer()
	v.clock = fakeClock(16 * time.Millisecond)
	for _, size := range []struct{ w, h int }{{40, 3}, {80, 10}, {120, 30}} {
		out := v.View(th, size.w, size.h)
		if got := len(strings.Split(out, "\n")); got != size.h {
			t.Fatalf("View(%d, %d) rendered %d rows, want %d", size.w, size.h, got, size.h)
		}
	}
}

// Peak-hold caps (MUS-16): when a bar drops, its cap must hold at the local
// maximum briefly and then fall with gravity — not snap down with the bar,
// and not linger for seconds (stale caps read as off-tempo).
func TestMiniVizPeakCapsHoldThenFall(t *testing.T) {
	th := theme.FromName("")
	v := NewMiniVisualizer()
	v.clock = fakeClock(16 * time.Millisecond) // steady 62.5fps

	// First render sizes the internal per-bar state for this geometry.
	_ = v.View(th, 40, 6)

	// Spike one bar. Not playing → each following frame decays the bar by
	// 0.85x, which is much faster than the cap comes down.
	v.heights[4] = 0.4
	_ = v.View(th, 40, 6)
	capAfterSpike := v.capPos[4]
	if capAfterSpike < 0.5 { // 0.4 decayed once then ×2 draw gain ≈ 0.68
		t.Fatalf("cap did not rise with the bar: capPos = %v", capAfterSpike)
	}

	// ~80ms later (5 frames): inside the 120ms hold window, bar collapsed.
	for i := 0; i < 5; i++ {
		_ = v.View(th, 40, 6)
	}
	drawn := clamp01(v.heights[4] * v.peakGain)
	if v.capPos[4] <= drawn {
		t.Fatalf("cap fell with the bar instead of holding: cap = %v, bar = %v",
			v.capPos[4], drawn)
	}
	if v.capPos[4] > capAfterSpike {
		t.Fatalf("cap rose without a new peak: %v > %v", v.capPos[4], capAfterSpike)
	}

	// Within ~one beat at a typical tempo (0.12s hold + accelerating fall
	// ≈ 0.6s total) the cap must have come back down — a full second of
	// frames is comfortably past that.
	for i := 0; i < 63; i++ {
		_ = v.View(th, 40, 6)
	}
	if v.capPos[4] > 0.05 {
		t.Fatalf("cap still hovering after ~1s: capPos = %v", v.capPos[4])
	}
}

// The CAVA coefficients must follow the MEASURED frame time, not an assumed
// 60fps: when frames arrive at half speed, framerate_mod must double, so the
// filters cover the same real time per second of music (MUS-16 rework — the
// motion drifting off-tempo when the render cadence varies).
func TestMiniVizCoefficientsTrackRealFrameTime(t *testing.T) {
	th := theme.FromName("")

	v60 := NewMiniVisualizer()
	v60.clock = fakeClock(16667 * time.Microsecond)
	_ = v60.View(th, 40, 4) // first frame establishes the baseline
	_ = v60.View(th, 40, 4)

	v30 := NewMiniVisualizer()
	v30.clock = fakeClock(33333 * time.Microsecond)
	_ = v30.View(th, 40, 4)
	_ = v30.View(th, 40, 4)

	if v60.framerateMod <= 0 || v30.framerateMod <= 0 {
		t.Fatalf("framerateMod not derived: 60fps=%v 30fps=%v", v60.framerateMod, v30.framerateMod)
	}
	ratio := v30.framerateMod / v60.framerateMod
	if ratio < 1.9 || ratio > 2.1 {
		t.Fatalf("framerate_mod must scale with real dt: 30fps/60fps ratio = %v, want ≈2", ratio)
	}
}

// MUSICTUI_VIZ_CAPS=0 must remove the cap dots from the render entirely.
func TestMiniVizCapsEnvDisable(t *testing.T) {
	t.Setenv("MUSICTUI_VIZ_CAPS", "0")
	v := NewMiniVisualizer()
	if !v.capsOff {
		t.Fatal("MUSICTUI_VIZ_CAPS=0 did not disable caps")
	}
}
