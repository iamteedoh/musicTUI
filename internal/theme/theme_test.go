package theme

import (
	"math"
	"testing"
)

// testLinear/testLuma/contrastRatio are an independent WCAG oracle — kept
// separate from the package's own lstar math so a bug there can't hide a
// contrast regression here.
func testLinear(c uint8) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func testLuma(hex string) float64 {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		panic("bad hex in test: " + hex)
	}
	return 0.2126*testLinear(r) + 0.7152*testLinear(g) + 0.0722*testLinear(b)
}

func contrastRatio(a, b string) float64 {
	la, lb := testLuma(a), testLuma(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func allThemes() []Theme {
	out := make([]Theme, 0, len(AllThemes))
	for _, k := range AllThemes {
		out = append(out, FromName(k))
	}
	return out
}

func TestRegistryHasTwelvePlusThemesAcrossTiers(t *testing.T) {
	if len(AllThemes) < 12 {
		t.Fatalf("registry has %d themes, MUS-32 wants about 12", len(AllThemes))
	}
	counts := map[Tier]int{}
	for _, th := range allThemes() {
		counts[th.Tier]++
	}
	for _, tier := range []Tier{TierDark, TierMedium, TierLight} {
		if counts[tier] < 4 {
			t.Errorf("tier %s has %d themes, want at least 4 (a third of ~12)", tier, counts[tier])
		}
	}
}

func TestFromNameResolvesEveryRegisteredKey(t *testing.T) {
	seen := map[string]bool{}
	for _, k := range AllThemes {
		th := FromName(k)
		if th.Name == "" || th.Tier == "" {
			t.Errorf("FromName(%q) has empty Name or Tier", k)
		}
		if seen[th.Name] {
			t.Errorf("duplicate theme name %q — the view-cache fingerprint keys off Name", th.Name)
		}
		seen[th.Name] = true
	}
	// Every key except nord must resolve to something other than the Nord
	// fallback, otherwise a registry typo silently maps a theme to Nord.
	for _, k := range AllThemes {
		if k != "nord" && FromName(k).Name == "Nord" {
			t.Errorf("FromName(%q) fell through to the Nord fallback", k)
		}
	}
}

func TestFromNameAliasesAndFallback(t *testing.T) {
	cases := map[string]string{
		"tokyo-night":    "Tokyo Night",
		"tokyonight":     "Tokyo Night",
		"TOKYO_NIGHT":    "Tokyo Night",
		"solarized_dark": "Solarized Dark",
		"monokai":        "Monokai Pro",
		"rose-pine-moon": "Rosé Pine Moon",
		"latte":          "Catppuccin Latte",
		"github":         "GitHub Light",
		"catppuccin":     "Catppuccin Mocha",
		"gruvbox":        "Gruvbox Dark",
		"":               "Nord",
		"no-such-theme":  "Nord",
	}
	for in, want := range cases {
		if got := FromName(in).Name; got != want {
			t.Errorf("FromName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOptionsStartWithAutoAndCoverRegistry(t *testing.T) {
	opts := Options()
	if opts[0] != Auto {
		t.Fatalf("Options()[0] = %q, want %q", opts[0], Auto)
	}
	if len(opts) != len(AllThemes)+1 {
		t.Fatalf("Options() has %d entries, want %d", len(opts), len(AllThemes)+1)
	}
}

// terminalBg is the background a theme is actually rendered on — the
// TERMINAL's, not th.Bg, because musicTUI never paints a full-screen
// background (see panel.go fillLine). A dark-tier user's terminal is
// near-black and a light-tier user's is near-white, so those are the honest
// worst cases to measure against; mid-tone terminals vary too much to assume,
// so the medium tier is held to its own Bg.
func terminalBg(th Theme) string {
	switch th.Tier {
	case TierDark:
		return "#000000"
	case TierLight:
		return "#FFFFFF"
	default:
		return string(th.Bg)
	}
}

// The bug behind MUS-32: palettes whose text vanishes against the terminal
// background they claim to support.
//
// FgMuted is included deliberately. The name oversells it as decoration — it
// is the app's second most-used text color and renders real content (track
// numbers, durations, column headers, position indicators), so it has to stay
// readable. The dark and medium tiers keep their long-shipped comment-gray
// values, which are faint by design and accepted; the light tier is held to a
// real floor because the same trick (a mid-gray a few steps off the
// background) is far harsher as light-gray-on-white than as dark-gray-on-black.
func TestEveryThemeIsLegibleOnItsTerminalBackground(t *testing.T) {
	mutedMin := func(th Theme) float64 {
		if th.Tier == TierLight {
			return 4.0
		}
		return 2.3 // grandfathered: the shipped dark/medium look
	}
	minima := []struct {
		role string
		get  func(Theme) string
		min  func(Theme) float64
	}{
		{"Fg", func(th Theme) string { return string(th.Fg) }, func(Theme) float64 { return 4.5 }},
		{"FgDim", func(th Theme) string { return string(th.FgDim) }, func(Theme) float64 { return 3.0 }},
		{"FgMuted", func(th Theme) string { return string(th.FgMuted) }, mutedMin},
		{"Accent", func(th Theme) string { return string(th.Accent) }, func(Theme) float64 { return 2.5 }},
		{"Error", func(th Theme) string { return string(th.Error) }, func(Theme) float64 { return 2.5 }},
	}
	for _, th := range allThemes() {
		bg := terminalBg(th)
		for _, m := range minima {
			want := m.min(th)
			if got := contrastRatio(m.get(th), bg); got < want {
				t.Errorf("%s (%s tier): %s %s on a %s terminal has contrast %.2f, want >= %.2f",
					th.Name, th.Tier, m.role, m.get(th), bg, got, want)
			}
		}
	}
}

// Text must keep a visible hierarchy: primary reads stronger than dimmed,
// dimmed stronger than muted. Inverting this is how a palette ends up with
// its *titles* fainter than its *track numbers* — the exact inversion users
// saw when a dark theme was rendered on a light terminal.
func TestTextHierarchyIsOrdered(t *testing.T) {
	for _, th := range allThemes() {
		bg := terminalBg(th)
		fg := contrastRatio(string(th.Fg), bg)
		dim := contrastRatio(string(th.FgDim), bg)
		muted := contrastRatio(string(th.FgMuted), bg)
		if !(fg > dim && dim > muted) {
			t.Errorf("%s: contrast order Fg %.2f > FgDim %.2f > FgMuted %.2f is broken on a %s terminal",
				th.Name, fg, dim, muted, bg)
		}
	}
}

// Selection rows paint Background(SurfaceBright) under Fg text; the search
// cursor inverts Fg/Bg. Both must stay readable in every theme.
func TestSelectionAndCursorContrast(t *testing.T) {
	for _, th := range allThemes() {
		if got := contrastRatio(string(th.Fg), string(th.SurfaceBright)); got < 3.0 {
			t.Errorf("%s: Fg on SurfaceBright selection has contrast %.2f, want >= 3.0", th.Name, got)
		}
		if got := contrastRatio(string(th.Bg), string(th.Fg)); got < 4.5 {
			t.Errorf("%s: cursor (Bg on Fg) has contrast %.2f, want >= 4.5", th.Name, got)
		}
	}
}

func TestTierForBackground(t *testing.T) {
	cases := []struct {
		hex  string
		want Tier
		ok   bool
	}{
		{"#000000", TierDark, true},
		{"#1A1B26", TierDark, true}, // Tokyo Night bg
		{"#2E3440", TierDark, true}, // Nord bg
		{"#808080", TierMedium, true},
		{"#666666", TierMedium, true},
		{"#FFFFFF", TierLight, true},
		{"#FDF6E3", TierLight, true}, // Solarized Light bg
		{"#EFF1F5", TierLight, true}, // Latte bg
		{"", TierDark, false},
		{"not-a-color", TierDark, false},
		{"#12345", TierDark, false},
	}
	for _, c := range cases {
		got, ok := TierForBackground(c.hex)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("TierForBackground(%q) = (%s, %t), want (%s, %t)", c.hex, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveExplicitNameIgnoresBackground(t *testing.T) {
	if got := Resolve("dracula", "#FFFFFF").Name; got != "Dracula" {
		t.Fatalf("Resolve(dracula, white bg) = %q, an explicit choice must win", got)
	}
}

func TestResolveAutoPicksTierDefaults(t *testing.T) {
	t.Setenv("COLORFGBG", "") // isolate from the host terminal
	cases := []struct {
		bg   string
		want string
	}{
		{"#101010", "Nord"},
		{"#808080", "Monokai Pro"},
		{"#FDF6E3", "Catppuccin Latte"},
		{"", "Nord"}, // no signal at all → the historical dark default
	}
	for _, c := range cases {
		if got := Resolve(Auto, c.bg).Name; got != c.want {
			t.Errorf("Resolve(auto, %q) = %q, want %q", c.bg, got, c.want)
		}
		// Empty name means the config predates the theme key — same as auto.
		if got := Resolve("", c.bg).Name; got != c.want {
			t.Errorf("Resolve(\"\", %q) = %q, want %q", c.bg, got, c.want)
		}
	}
}

func TestResolveAutoFallsBackToColorFgBg(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"0;15", "Catppuccin Latte"},          // light bg
		{"15;0", "Nord"},                      // dark bg
		{"12;default;15", "Catppuccin Latte"}, // rxvt three-field form
		{"garbage", "Nord"},
		{"", "Nord"},
	}
	for _, c := range cases {
		t.Setenv("COLORFGBG", c.env)
		if got := Resolve(Auto, "").Name; got != c.want {
			t.Errorf("Resolve(auto) with COLORFGBG=%q = %q, want %q", c.env, got, c.want)
		}
	}
}

// The probe's background answer must beat the coarse COLORFGBG hint.
func TestResolveProbeBeatsColorFgBg(t *testing.T) {
	t.Setenv("COLORFGBG", "15;0") // env claims dark
	if got := Resolve(Auto, "#FFFFFF").Name; got != "Catppuccin Latte" {
		t.Fatalf("Resolve(auto, #FFFFFF) = %q, want the probe's light answer to win", got)
	}
}
