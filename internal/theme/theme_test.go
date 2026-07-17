package theme

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
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

// ── MUS-36: background contrast adaptation ──────────────────────────────────

// Realistic dark terminal backgrounds that are NOT pure black — the case the
// fixed dark palette rendered illegibly (the reported Ghostty-navy bug).
var navyBackgrounds = []string{
	"#0D1117", // GitHub dark
	"#0F172A", // slate navy
	"#111318", // near-black blue-gray
	"#1A1B26", // Tokyo Night
	"#1D1F21", // Ghostty/iTerm default-ish
	"#1E2C48", // deep navy
	"#282C34", // One Dark
	"#2E3440", // Nord's own bg
}

func mustColor(hex string) colorful.Color {
	c, err := colorful.Hex(strings.ToLower(hex))
	if err != nil {
		panic("bad hex in test: " + hex)
	}
	return c
}

// A terminal dark or light enough that every role already clears its floor must
// resolve BYTE-FOR-BYTE to the raw tier palette: adaptation is a pure no-op, so
// existing near-black (and near-white) users see zero change.
func TestResolveIsNoOpWhenPaletteAlreadyLegible(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	if got := Resolve(Auto, "#000000"); !reflect.DeepEqual(got, Nord()) {
		t.Errorf("Resolve(auto, #000000) must equal raw Nord() byte-for-byte;\n got %+v\nwant %+v", got, Nord())
	}
	if got := Resolve(Auto, "#FFFFFF"); !reflect.DeepEqual(got, CatppuccinLatte()) {
		t.Errorf("Resolve(auto, #FFFFFF) must equal raw Catppuccin Latte() byte-for-byte")
	}
}

// A silent terminal (no OSC 11 answer) or a garbage background must fall through
// to today's behavior — never a half-adapted palette.
func TestResolveUnknownBackgroundFallsBack(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, bg := range []string{"", "not-a-color", "#12345"} {
		if got := Resolve(Auto, bg); !reflect.DeepEqual(got, Nord()) {
			t.Errorf("Resolve(auto, %q) must fall back to raw Nord()", bg)
		}
	}
}

// The reported bug: muted text (track numbers, durations, column headers) must
// be legible on a navy terminal. Every body-text role is held to its floor,
// measured by the test's INDEPENDENT WCAG oracle so the adapted colors are
// checked by a second implementation.
func TestResolveMutedIsLegibleOnNavy(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, bg := range navyBackgrounds {
		th := Resolve(Auto, bg)
		checks := []struct {
			role string
			hex  string
			min  float64
		}{
			{"Fg", string(th.Fg), 4.5},
			{"FgDim", string(th.FgDim), 3.0},
			{"FgMuted", string(th.FgMuted), 2.7},
		}
		for _, c := range checks {
			// Small slack absorbs 8-bit rounding at the exact floor.
			if got := contrastRatio(c.hex, bg); got < c.min-0.02 {
				t.Errorf("Resolve(auto, %s): %s %s contrast %.2f, want >= %.2f",
					bg, c.role, c.hex, got, c.min)
			}
		}
	}
}

// Adaptation must never invert the visible text hierarchy on ANY background,
// including the compressed L* 38–65 mid band where roles crowd together.
func TestResolveKeepsHierarchyOnEveryBackground(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	bgs := append([]string{}, navyBackgrounds...)
	// mid-grays, light-grays (where the light muted floor once chased an
	// unreachable target), a saturated bright bg, and the near-white extremes.
	bgs = append(bgs, "#3B3B3B", "#5B5B5B", "#6E6E6E", "#7C7C7C", "#909090",
		"#B0B0B0", "#B2B2B2", "#C0C0C0", "#00C000", "#FDF6E3", "#FFFFFF")
	for _, bg := range bgs {
		th := Resolve(Auto, bg)
		fg := contrastRatio(string(th.Fg), bg)
		dim := contrastRatio(string(th.FgDim), bg)
		muted := contrastRatio(string(th.FgMuted), bg)
		if !(fg > dim && dim > muted) {
			t.Errorf("Resolve(auto, %s): hierarchy broken — Fg %.2f > FgDim %.2f > FgMuted %.2f",
				bg, fg, dim, muted)
		}
	}
}

// Cross-tier stress: every EXPLICIT theme, on both a dark and a light terminal,
// must keep its body text legible and the Fg>FgDim>FgMuted order intact — even
// when the palette's own luminance order is inverted for that terminal (a light
// theme on a dark terminal, which is what the reported github_light bug was).
func TestResolveExplicitThemesLegibleAndOrderedCrossTier(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, key := range AllThemes {
		for _, bg := range []string{"#1A1B26", "#0D1117", "#FFFFFF", "#EFF1F5"} {
			th := Resolve(key, bg)
			fg := contrastRatio(string(th.Fg), bg)
			dim := contrastRatio(string(th.FgDim), bg)
			muted := contrastRatio(string(th.FgMuted), bg)
			if fg < 4.5-0.05 || dim < 3.0-0.05 || muted < 2.7-0.05 {
				t.Errorf("%s on %s: below floors — Fg %.2f FgDim %.2f FgMuted %.2f", key, bg, fg, dim, muted)
			}
			if !(fg > dim && dim > muted) {
				t.Errorf("%s on %s: hierarchy broken — Fg %.2f > FgDim %.2f > FgMuted %.2f", key, bg, fg, dim, muted)
			}
		}
	}
}

// The mid-gray win adaptation has over a fixed-palette retune: a true mid-tone
// terminal routes to the medium tier and its muted role is lifted to legibility
// too — something no single fixed value can do across the whole band.
func TestResolveMutedIsLegibleOnMidGray(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, bg := range []string{"#6E6E6E", "#7C7C7C", "#909090"} {
		th := Resolve(Auto, bg)
		if got := contrastRatio(string(th.FgMuted), bg); got < 2.7-0.02 {
			t.Errorf("Resolve(auto, %s): FgMuted contrast %.2f, want >= 2.7", bg, got)
		}
	}
}

// An explicitly-chosen theme keeps its identity (the user picked WHICH palette)
// but is still contrast-adapted to the terminal so its text stays readable — a
// no-op when it already matches, a rescue when it doesn't (MUS-36).
func TestResolveExplicitThemeAdaptsButKeepsIdentity(t *testing.T) {
	t.Setenv("COLORFGBG", "")

	// Matching terminal → returned byte-for-byte (the pick is honored exactly).
	if got := Resolve("nord", "#000000"); !reflect.DeepEqual(got, Nord()) {
		t.Error("explicit nord on pure black must equal raw Nord() — already legible, no change")
	}
	if got := Resolve("github_light", "#FFFFFF"); !reflect.DeepEqual(got, GitHubLight()) {
		t.Error("explicit github_light on white must equal raw GitHubLight() — already legible")
	}

	// The reported case: a light theme on a DARK terminal. Identity is kept, but
	// its near-black body text is lifted to readable contrast.
	dark := "#1A1B26"
	got := Resolve("github_light", dark)
	if got.Name != "GitHub Light" || got.Tier != TierLight {
		t.Fatalf("explicit theme lost its identity: got %q (%s)", got.Name, got.Tier)
	}
	if c := contrastRatio(string(got.Fg), dark); c < 4.5-0.05 {
		t.Errorf("github_light Fg on a dark terminal has contrast %.2f, want >= 4.5 (rescued)", c)
	}
	if c := contrastRatio(string(got.FgMuted), dark); c < 2.7-0.05 {
		t.Errorf("github_light FgMuted on a dark terminal has contrast %.2f, want >= 2.7 (rescued)", c)
	}
	if raw := GitHubLight(); string(got.Fg) == string(raw.Fg) {
		t.Error("github_light Fg was not adapted on a dark terminal — text would be invisible")
	}

	// An explicit DARK theme on a navy terminal also gets the muted-text rescue.
	if c := contrastRatio(string(Resolve("nord", "#1E2C48").FgMuted), "#1E2C48"); c < 2.7-0.05 {
		t.Errorf("explicit nord FgMuted on navy has contrast %.2f, want >= 2.7 (rescued)", c)
	}
}

// raiseContrast contract, verified on the 8-bit color actually emitted:
// already-passing input is returned unchanged; a failing input is lifted to at
// least the floor after rounding; an impossible target is driven to a pole.
func TestRaiseContrastReachesFloorOnEmittedColor(t *testing.T) {
	navy := mustColor("#1E2C48")

	// Nord FgMuted starts ~1.9 on this navy → must reach 2.7 after rounding.
	adj := raiseContrast(mustColor("#4C566A"), navy, 2.7)
	if got := wcagContrast(mustColor(adj.Hex()), navy); got < 2.7 {
		t.Errorf("raiseContrast: emitted contrast %.3f, want >= 2.7", got)
	}

	// An already-legible role is returned byte-identical (no needless shift).
	fg := mustColor("#d8dee9")
	if out := raiseContrast(fg, navy, 4.5); out.Hex() != fg.Hex() {
		t.Errorf("raiseContrast shifted an already-passing color: %s -> %s", fg.Hex(), out.Hex())
	}

	// Impossible target (far above any achievable contrast) → a pure pole.
	gray := mustColor("#7C7C7C")
	pole := raiseContrast(gray, gray, 99)
	if h := pole.Hex(); h != "#000000" && h != "#ffffff" {
		t.Errorf("raiseContrast: unreachable target should return a pole, got %s", h)
	}
}

// Dark terminals whose background sits close to the palette's selection block,
// where the highlight fades into the surrounding rows (the follow-up MUS-36
// asked to fix alongside the muted-text bug).
var midDarkBackgrounds = []string{"#282C34", "#303540", "#383E48", "#3B4048"}

// On a mid-dark terminal the selection block must be re-seated so it (a) stands
// out from the terminal background at least as well as the raw palette would and
// clears a visible-separation floor, and (b) keeps the primary accent selection
// text legible.
func TestResolveSelectionBlockStaysVisibleOnMidDark(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	rawSB := string(Nord().SurfaceBright)
	rawAccent := string(Nord().Accent)
	for _, bg := range midDarkBackgrounds {
		th := Resolve(Auto, bg)
		sepRaw := contrastRatio(rawSB, bg)
		sepNew := contrastRatio(string(th.SurfaceBright), bg)
		if sepNew < sepRaw-0.01 {
			t.Errorf("bg %s: adapted selection block sep %.2f is worse than raw %.2f", bg, sepNew, sepRaw)
		}
		if sepNew < 1.45 {
			t.Errorf("bg %s: selection block sep %.2f still fades (want >= ~1.5)", bg, sepNew)
		}
		if acc := contrastRatio(rawAccent, string(th.SurfaceBright)); acc < 3.0-0.05 {
			t.Errorf("bg %s: accent text on the selection block dropped to %.2f (want >= 3.0)", bg, acc)
		}
	}
}

// A dark terminal whose selection block is already distinct (near-black, and the
// reported Ghostty navy) must leave SurfaceBright untouched — no needless shift.
func TestResolveSelectionBlockUntouchedWhenAlreadyDistinct(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, bg := range []string{"#000000", "#0D1117", "#1D1F21", "#1A1B26"} {
		if got, want := string(Resolve(Auto, bg).SurfaceBright), string(Nord().SurfaceBright); got != want {
			t.Errorf("bg %s: SurfaceBright changed to %s, want the raw %s (already distinct)", bg, got, want)
		}
	}
}

// The light tier's selection block is deliberately low-contrast and must never
// be adapted here (doing so once inverted a light block to near-black on white).
func TestResolveLightTierSelectionBlockUntouched(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	for _, bg := range []string{"#FFFFFF", "#EFF1F5", "#E0E0E0", "#D8DCE4"} {
		if got, want := string(Resolve(Auto, bg).SurfaceBright), string(CatppuccinLatte().SurfaceBright); got != want {
			t.Errorf("bg %s (light tier): SurfaceBright changed to %s, want the raw %s", bg, got, want)
		}
	}
}

// fitSelectionSurface contract: an already-distinct block is returned unchanged;
// a faded one is re-seated to clear the separation and text floors it can.
func TestFitSelectionSurface(t *testing.T) {
	accent := mustColor("#88c0d0") // Nord accent
	dim := mustColor("#5e81ac")    // Nord accentDim
	texts := []textReq{{accent, 3.0}, {dim, 2.3}}
	sb := mustColor("#3b4252")

	// Already distinct against near-black → unchanged.
	if got := fitSelectionSurface(sb, mustColor("#000000"), texts, 1.5); got.Hex() != sb.Hex() {
		t.Errorf("distinct block was shifted: %s -> %s", sb.Hex(), got.Hex())
	}
	// Faded against a mid-dark bg → re-seated to clear the separation floor with
	// accent text still legible.
	bg := mustColor("#383e48")
	got := fitSelectionSurface(sb, bg, texts, 1.5)
	if wcagContrast(mustColor(got.Hex()), bg) < 1.45 {
		t.Errorf("re-seated block sep %.2f, want >= ~1.5", wcagContrast(mustColor(got.Hex()), bg))
	}
	if wcagContrast(accent, mustColor(got.Hex())) < 3.0 {
		t.Errorf("accent on re-seated block %.2f, want >= 3.0", wcagContrast(accent, mustColor(got.Hex())))
	}
}
