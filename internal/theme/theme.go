package theme

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Tier buckets a theme by the terminal background brightness it is designed
// for. musicTUI never paints the full screen — panels use the terminal's own
// background — so a palette only reads well when its foregrounds match the
// brightness of the background underneath it. Auto-detection picks the tier
// from the terminal's reported background color; the settings picker walks
// the list dark → medium → light.
type Tier string

const (
	TierDark   Tier = "dark"
	TierMedium Tier = "medium"
	TierLight  Tier = "light"
)

type Theme struct {
	Name          string
	Tier          Tier
	BaseBg        lipgloss.Color // Darkest background — outer frame and panel gaps
	Bg            lipgloss.Color
	Fg            lipgloss.Color
	FgDim         lipgloss.Color
	FgMuted       lipgloss.Color
	Surface       lipgloss.Color
	SurfaceBright lipgloss.Color
	Accent        lipgloss.Color
	AccentDim     lipgloss.Color
	Success       lipgloss.Color
	Warning       lipgloss.Color
	Error         lipgloss.Color
	SidebarBg     lipgloss.Color
	SidebarSel    lipgloss.Color
	NowPlayingBg  lipgloss.Color
	Border        lipgloss.Color
	BorderFocused lipgloss.Color
	ProgressBar   lipgloss.Color
	ProgressBg    lipgloss.Color
}

// ── Dark tier ──────────────────────────────────────────────────────────────

func Nord() Theme {
	return Theme{
		Name:          "Nord",
		Tier:          TierDark,
		BaseBg:        lipgloss.Color("#000000"),
		Bg:            lipgloss.Color("#2E3440"),
		Fg:            lipgloss.Color("#D8DEE9"),
		FgDim:         lipgloss.Color("#B2B2BE"),
		FgMuted:       lipgloss.Color("#4C566A"),
		Surface:       lipgloss.Color("#2E3440"),
		SurfaceBright: lipgloss.Color("#3B4252"),
		Accent:        lipgloss.Color("#88C0D0"),
		AccentDim:     lipgloss.Color("#5E81AC"),
		Success:       lipgloss.Color("#A3BE8C"),
		Warning:       lipgloss.Color("#EBCB8B"),
		Error:         lipgloss.Color("#BF616A"),
		SidebarBg:     lipgloss.Color("#0C0E13"),
		SidebarSel:    lipgloss.Color("#2E3440"),
		NowPlayingBg:  lipgloss.Color("#2E3440"),
		Border:        lipgloss.Color("#88C0D0"),
		BorderFocused: lipgloss.Color("#ECEFF4"),
		ProgressBar:   lipgloss.Color("#88C0D0"),
		ProgressBg:    lipgloss.Color("#2E3440"),
	}
}

func Dracula() Theme {
	return Theme{
		Name:          "Dracula",
		Tier:          TierDark,
		BaseBg:        lipgloss.Color("#000000"),
		Bg:            lipgloss.Color("#282A36"),
		Fg:            lipgloss.Color("#F8F8F2"),
		FgDim:         lipgloss.Color("#BD93F9"),
		FgMuted:       lipgloss.Color("#6272A4"),
		Surface:       lipgloss.Color("#282A36"),
		SurfaceBright: lipgloss.Color("#44475A"),
		Accent:        lipgloss.Color("#8BE9FD"),
		AccentDim:     lipgloss.Color("#BD93F9"),
		Success:       lipgloss.Color("#50FA7B"),
		Warning:       lipgloss.Color("#F1FA8C"),
		Error:         lipgloss.Color("#FF5555"),
		SidebarBg:     lipgloss.Color("#0D0E14"),
		SidebarSel:    lipgloss.Color("#282A36"),
		NowPlayingBg:  lipgloss.Color("#282A36"),
		Border:        lipgloss.Color("#8BE9FD"),
		BorderFocused: lipgloss.Color("#FF79C6"),
		ProgressBar:   lipgloss.Color("#FF79C6"),
		ProgressBg:    lipgloss.Color("#282A36"),
	}
}

func Catppuccin() Theme {
	return Theme{
		Name:          "Catppuccin Mocha",
		Tier:          TierDark,
		BaseBg:        lipgloss.Color("#000000"),
		Bg:            lipgloss.Color("#1E1E2E"),
		Fg:            lipgloss.Color("#CDD6F4"),
		FgDim:         lipgloss.Color("#A6ADC8"),
		FgMuted:       lipgloss.Color("#585B70"),
		Surface:       lipgloss.Color("#1E1E2E"),
		SurfaceBright: lipgloss.Color("#313244"),
		Accent:        lipgloss.Color("#89B4FA"),
		AccentDim:     lipgloss.Color("#74C7EC"),
		Success:       lipgloss.Color("#A6E3A1"),
		Warning:       lipgloss.Color("#F9E2AF"),
		Error:         lipgloss.Color("#F38BA8"),
		SidebarBg:     lipgloss.Color("#080810"),
		SidebarSel:    lipgloss.Color("#1E1E2E"),
		NowPlayingBg:  lipgloss.Color("#1E1E2E"),
		Border:        lipgloss.Color("#89B4FA"),
		BorderFocused: lipgloss.Color("#CBA6F7"),
		ProgressBar:   lipgloss.Color("#CBA6F7"),
		ProgressBg:    lipgloss.Color("#313244"),
	}
}

func Gruvbox() Theme {
	return Theme{
		Name:          "Gruvbox Dark",
		Tier:          TierDark,
		BaseBg:        lipgloss.Color("#000000"),
		Bg:            lipgloss.Color("#282828"),
		Fg:            lipgloss.Color("#EBDBB2"),
		FgDim:         lipgloss.Color("#BDAE93"),
		FgMuted:       lipgloss.Color("#665C54"),
		Surface:       lipgloss.Color("#282828"),
		SurfaceBright: lipgloss.Color("#3C3836"),
		Accent:        lipgloss.Color("#FABD2F"),
		AccentDim:     lipgloss.Color("#D65D0E"),
		Success:       lipgloss.Color("#B8BB26"),
		Warning:       lipgloss.Color("#FABD2F"),
		Error:         lipgloss.Color("#FB4934"),
		SidebarBg:     lipgloss.Color("#080808"),
		SidebarSel:    lipgloss.Color("#282828"),
		NowPlayingBg:  lipgloss.Color("#282828"),
		Border:        lipgloss.Color("#FABD2F"),
		BorderFocused: lipgloss.Color("#FABD2F"),
		ProgressBar:   lipgloss.Color("#FABD2F"),
		ProgressBg:    lipgloss.Color("#3C3836"),
	}
}

func TokyoNight() Theme {
	return Theme{
		Name:          "Tokyo Night",
		Tier:          TierDark,
		BaseBg:        lipgloss.Color("#000000"),
		Bg:            lipgloss.Color("#1A1B26"),
		Fg:            lipgloss.Color("#A9B1D6"),
		FgDim:         lipgloss.Color("#8289AA"),
		FgMuted:       lipgloss.Color("#414868"),
		Surface:       lipgloss.Color("#1A1B26"),
		SurfaceBright: lipgloss.Color("#24283B"),
		Accent:        lipgloss.Color("#7AA2F7"),
		AccentDim:     lipgloss.Color("#7DCFFF"),
		Success:       lipgloss.Color("#9ECE6A"),
		Warning:       lipgloss.Color("#E0AF68"),
		Error:         lipgloss.Color("#F7768E"),
		SidebarBg:     lipgloss.Color("#07080E"),
		SidebarSel:    lipgloss.Color("#1A1B26"),
		NowPlayingBg:  lipgloss.Color("#1A1B26"),
		Border:        lipgloss.Color("#7AA2F7"),
		BorderFocused: lipgloss.Color("#BB9AF7"),
		ProgressBar:   lipgloss.Color("#BB9AF7"),
		ProgressBg:    lipgloss.Color("#24283B"),
	}
}

// ── Medium tier ─────────────────────────────────────────────────────────────
// Softened palettes for mid-tone terminal backgrounds: brighter frames than
// the dark tier and no pure-black anywhere, so they sit comfortably on
// backgrounds that are neither near-black nor light.

func SolarizedDark() Theme {
	return Theme{
		Name:          "Solarized Dark",
		Tier:          TierMedium,
		BaseBg:        lipgloss.Color("#002B36"),
		Bg:            lipgloss.Color("#073642"),
		Fg:            lipgloss.Color("#93A1A1"),
		FgDim:         lipgloss.Color("#839496"),
		FgMuted:       lipgloss.Color("#586E75"),
		Surface:       lipgloss.Color("#073642"),
		SurfaceBright: lipgloss.Color("#0E4B5A"),
		Accent:        lipgloss.Color("#268BD2"),
		AccentDim:     lipgloss.Color("#2AA198"),
		Success:       lipgloss.Color("#859900"),
		Warning:       lipgloss.Color("#B58900"),
		Error:         lipgloss.Color("#DC322F"),
		SidebarBg:     lipgloss.Color("#002B36"),
		SidebarSel:    lipgloss.Color("#073642"),
		NowPlayingBg:  lipgloss.Color("#073642"),
		Border:        lipgloss.Color("#268BD2"),
		BorderFocused: lipgloss.Color("#B58900"),
		ProgressBar:   lipgloss.Color("#2AA198"),
		ProgressBg:    lipgloss.Color("#0E4B5A"),
	}
}

func Everforest() Theme {
	return Theme{
		Name:          "Everforest",
		Tier:          TierMedium,
		BaseBg:        lipgloss.Color("#232A2E"),
		Bg:            lipgloss.Color("#2D353B"),
		Fg:            lipgloss.Color("#D3C6AA"),
		FgDim:         lipgloss.Color("#9DA9A0"),
		FgMuted:       lipgloss.Color("#7A8478"),
		Surface:       lipgloss.Color("#2D353B"),
		SurfaceBright: lipgloss.Color("#3D484D"),
		Accent:        lipgloss.Color("#A7C080"),
		AccentDim:     lipgloss.Color("#83C092"),
		Success:       lipgloss.Color("#A7C080"),
		Warning:       lipgloss.Color("#DBBC7F"),
		Error:         lipgloss.Color("#E67E80"),
		SidebarBg:     lipgloss.Color("#232A2E"),
		SidebarSel:    lipgloss.Color("#2D353B"),
		NowPlayingBg:  lipgloss.Color("#2D353B"),
		Border:        lipgloss.Color("#A7C080"),
		BorderFocused: lipgloss.Color("#DBBC7F"),
		ProgressBar:   lipgloss.Color("#83C092"),
		ProgressBg:    lipgloss.Color("#3D484D"),
	}
}

func RosePineMoon() Theme {
	return Theme{
		Name:          "Rosé Pine Moon",
		Tier:          TierMedium,
		BaseBg:        lipgloss.Color("#232136"),
		Bg:            lipgloss.Color("#2A283E"),
		Fg:            lipgloss.Color("#E0DEF4"),
		FgDim:         lipgloss.Color("#908CAA"),
		FgMuted:       lipgloss.Color("#6E6A86"),
		Surface:       lipgloss.Color("#2A283E"),
		SurfaceBright: lipgloss.Color("#393552"),
		Accent:        lipgloss.Color("#C4A7E7"),
		AccentDim:     lipgloss.Color("#3E8FB0"),
		Success:       lipgloss.Color("#9CCFD8"),
		Warning:       lipgloss.Color("#F6C177"),
		Error:         lipgloss.Color("#EB6F92"),
		SidebarBg:     lipgloss.Color("#232136"),
		SidebarSel:    lipgloss.Color("#2A283E"),
		NowPlayingBg:  lipgloss.Color("#2A283E"),
		Border:        lipgloss.Color("#C4A7E7"),
		BorderFocused: lipgloss.Color("#EA9A97"),
		ProgressBar:   lipgloss.Color("#EA9A97"),
		ProgressBg:    lipgloss.Color("#393552"),
	}
}

func MonokaiPro() Theme {
	return Theme{
		Name:          "Monokai Pro",
		Tier:          TierMedium,
		BaseBg:        lipgloss.Color("#221F22"),
		Bg:            lipgloss.Color("#2D2A2E"),
		Fg:            lipgloss.Color("#FCFCFA"),
		FgDim:         lipgloss.Color("#C1C0C0"),
		FgMuted:       lipgloss.Color("#727072"),
		Surface:       lipgloss.Color("#2D2A2E"),
		SurfaceBright: lipgloss.Color("#403E41"),
		Accent:        lipgloss.Color("#FFD866"),
		AccentDim:     lipgloss.Color("#FC9867"),
		Success:       lipgloss.Color("#A9DC76"),
		Warning:       lipgloss.Color("#FFD866"),
		Error:         lipgloss.Color("#FF6188"),
		SidebarBg:     lipgloss.Color("#221F22"),
		SidebarSel:    lipgloss.Color("#2D2A2E"),
		NowPlayingBg:  lipgloss.Color("#2D2A2E"),
		Border:        lipgloss.Color("#FFD866"),
		BorderFocused: lipgloss.Color("#78DCE8"),
		ProgressBar:   lipgloss.Color("#AB9DF2"),
		ProgressBg:    lipgloss.Color("#403E41"),
	}
}

// ── Light tier ──────────────────────────────────────────────────────────────
// Dark foregrounds and deeper accents for light terminal backgrounds — the
// case the dark-only palette made illegible (MUS-32). Surface stays light so
// text-on-accent and the search cursor keep their inverse-video contrast.
//
// FgMuted matters more here than the name suggests: it is not decoration, it
// renders track numbers, durations and column headers. Each scheme's canonical
// "comment" gray (Solarized base1, Latte overlay0, GitHub fg.subtle) sits near
// 2.6:1 on a white terminal — fainter than any dark theme's muted text is on
// black — so the light tier steps one tone darker, staying inside each
// scheme's own palette.

func SolarizedLight() Theme {
	return Theme{
		Name:          "Solarized Light",
		Tier:          TierLight,
		BaseBg:        lipgloss.Color("#EEE8D5"),
		Bg:            lipgloss.Color("#FDF6E3"),
		Fg:            lipgloss.Color("#073642"), // base02
		FgDim:         lipgloss.Color("#586E75"), // base01
		FgMuted:       lipgloss.Color("#657B83"), // base00
		Surface:       lipgloss.Color("#FDF6E3"),
		SurfaceBright: lipgloss.Color("#EEE8D5"),
		Accent:        lipgloss.Color("#268BD2"),
		AccentDim:     lipgloss.Color("#2AA198"),
		Success:       lipgloss.Color("#859900"),
		Warning:       lipgloss.Color("#B58900"),
		Error:         lipgloss.Color("#DC322F"),
		SidebarBg:     lipgloss.Color("#EEE8D5"),
		SidebarSel:    lipgloss.Color("#FDF6E3"),
		NowPlayingBg:  lipgloss.Color("#FDF6E3"),
		Border:        lipgloss.Color("#268BD2"),
		BorderFocused: lipgloss.Color("#CB4B16"),
		ProgressBar:   lipgloss.Color("#268BD2"),
		ProgressBg:    lipgloss.Color("#EEE8D5"),
	}
}

func CatppuccinLatte() Theme {
	return Theme{
		Name:          "Catppuccin Latte",
		Tier:          TierLight,
		BaseBg:        lipgloss.Color("#DCE0E8"),
		Bg:            lipgloss.Color("#EFF1F5"),
		Fg:            lipgloss.Color("#4C4F69"), // text
		FgDim:         lipgloss.Color("#5C5F77"), // subtext1
		FgMuted:       lipgloss.Color("#6C6F85"), // subtext0
		Surface:       lipgloss.Color("#EFF1F5"),
		SurfaceBright: lipgloss.Color("#CCD0DA"),
		Accent:        lipgloss.Color("#1E66F5"),
		AccentDim:     lipgloss.Color("#209FB5"),
		Success:       lipgloss.Color("#40A02B"),
		Warning:       lipgloss.Color("#DF8E1D"),
		Error:         lipgloss.Color("#D20F39"),
		SidebarBg:     lipgloss.Color("#E6E9EF"),
		SidebarSel:    lipgloss.Color("#EFF1F5"),
		NowPlayingBg:  lipgloss.Color("#EFF1F5"),
		Border:        lipgloss.Color("#1E66F5"),
		BorderFocused: lipgloss.Color("#8839EF"),
		ProgressBar:   lipgloss.Color("#8839EF"),
		ProgressBg:    lipgloss.Color("#CCD0DA"),
	}
}

func GruvboxLight() Theme {
	return Theme{
		Name:          "Gruvbox Light",
		Tier:          TierLight,
		BaseBg:        lipgloss.Color("#EBDBB2"),
		Bg:            lipgloss.Color("#FBF1C7"),
		Fg:            lipgloss.Color("#3C3836"), // fg1
		FgDim:         lipgloss.Color("#504945"), // fg2
		FgMuted:       lipgloss.Color("#7C6F64"), // fg4
		Surface:       lipgloss.Color("#FBF1C7"),
		SurfaceBright: lipgloss.Color("#EBDBB2"),
		Accent:        lipgloss.Color("#B57614"),
		AccentDim:     lipgloss.Color("#AF3A03"),
		Success:       lipgloss.Color("#79740E"),
		Warning:       lipgloss.Color("#B57614"),
		Error:         lipgloss.Color("#9D0006"),
		SidebarBg:     lipgloss.Color("#EBDBB2"),
		SidebarSel:    lipgloss.Color("#FBF1C7"),
		NowPlayingBg:  lipgloss.Color("#FBF1C7"),
		Border:        lipgloss.Color("#B57614"),
		BorderFocused: lipgloss.Color("#AF3A03"),
		ProgressBar:   lipgloss.Color("#AF3A03"),
		ProgressBg:    lipgloss.Color("#EBDBB2"),
	}
}

func GitHubLight() Theme {
	return Theme{
		Name:          "GitHub Light",
		Tier:          TierLight,
		BaseBg:        lipgloss.Color("#EAEEF2"),
		Bg:            lipgloss.Color("#FFFFFF"),
		Fg:            lipgloss.Color("#24292F"), // fg.default
		FgDim:         lipgloss.Color("#57606A"), // fg.muted
		FgMuted:       lipgloss.Color("#6E7781"), // fg.subtle
		Surface:       lipgloss.Color("#FFFFFF"),
		SurfaceBright: lipgloss.Color("#EAEEF2"),
		Accent:        lipgloss.Color("#0969DA"),
		AccentDim:     lipgloss.Color("#218BFF"),
		Success:       lipgloss.Color("#1A7F37"),
		Warning:       lipgloss.Color("#9A6700"),
		Error:         lipgloss.Color("#CF222E"),
		SidebarBg:     lipgloss.Color("#F6F8FA"),
		SidebarSel:    lipgloss.Color("#FFFFFF"),
		NowPlayingBg:  lipgloss.Color("#FFFFFF"),
		Border:        lipgloss.Color("#0969DA"),
		BorderFocused: lipgloss.Color("#8250DF"),
		ProgressBar:   lipgloss.Color("#8250DF"),
		ProgressBg:    lipgloss.Color("#EAEEF2"),
	}
}

// registry orders every built-in theme dark → medium → light. The key is what
// config.toml stores and what the settings picker cycles through.
var registry = []struct {
	key   string
	build func() Theme
}{
	{"nord", Nord},
	{"dracula", Dracula},
	{"catppuccin", Catppuccin},
	{"gruvbox", Gruvbox},
	{"tokyo_night", TokyoNight},
	{"solarized", SolarizedDark},
	{"everforest", Everforest},
	{"rose_pine_moon", RosePineMoon},
	{"monokai_pro", MonokaiPro},
	{"solarized_light", SolarizedLight},
	{"catppuccin_latte", CatppuccinLatte},
	{"gruvbox_light", GruvboxLight},
	{"github_light", GitHubLight},
}

// AllThemes lists every built-in theme key in picker order.
var AllThemes = func() []string {
	keys := make([]string, len(registry))
	for i, e := range registry {
		keys[i] = e.key
	}
	return keys
}()

// Auto is the config value (and default) that means "match the terminal".
const Auto = "auto"

// Options is what the settings picker cycles through: Auto first, then every
// built-in theme dark → medium → light.
func Options() []string {
	return append([]string{Auto}, AllThemes...)
}

// aliases maps alternate spellings to registry keys, so hand-edited configs
// keep working. tokyo-night/tokyonight predate this table (MUS-32 kept them).
var aliases = map[string]string{
	"tokyo-night":      "tokyo_night",
	"tokyonight":       "tokyo_night",
	"solarized_dark":   "solarized",
	"solarized-dark":   "solarized",
	"rose-pine-moon":   "rose_pine_moon",
	"rosepine":         "rose_pine_moon",
	"monokai":          "monokai_pro",
	"monokai-pro":      "monokai_pro",
	"catppuccin_mocha": "catppuccin",
	"latte":            "catppuccin_latte",
	"github":           "github_light",
}

// FromName resolves an explicit theme key (or alias). Unknown names fall back
// to Nord, the historical default — auto-detection lives in Resolve, not here.
func FromName(name string) Theme {
	key := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := aliases[key]; ok {
		key = alias
	}
	for _, e := range registry {
		if e.key == key {
			return e.build()
		}
	}
	return Nord()
}

// tierDefault is the theme Resolve picks for each detected background tier.
func tierDefault(t Tier) Theme {
	switch t {
	case TierLight:
		return CatppuccinLatte()
	case TierMedium:
		return MonokaiPro()
	default:
		return Nord()
	}
}

// Resolve turns a configured theme name into a Theme. An explicit name chooses
// WHICH palette; "auto" (or empty) chooses one to match the terminal instead:
// termBg is the terminal's reported background color ("#rrggbb" from the termcap
// probe, empty when it didn't answer), with the COLORFGBG environment variable
// as a coarse fallback for terminals the probe can't reach (e.g. under tmux). No
// signal at all means dark — the assumption the app has always made.
//
// Either way, the resolved palette is contrast-adapted to termBg (MUS-36):
// legibility must not depend on how the theme was chosen. adaptToBackground is a
// no-op when the palette already reads well on termBg (or termBg is unknown), so
// an explicit theme that matches its terminal is returned exactly as authored;
// only a mismatch — e.g. an explicit light theme on a dark terminal — is rescued
// so its text stays readable.
func Resolve(name, termBg string) Theme {
	key := strings.ToLower(strings.TrimSpace(name))
	if key != "" && key != Auto {
		return adaptToBackground(FromName(name), termBg)
	}
	if t, ok := TierForBackground(termBg); ok {
		return adaptToBackground(tierDefault(t), termBg)
	}
	if t, ok := tierFromColorFgBg(os.Getenv("COLORFGBG")); ok {
		return tierDefault(t)
	}
	return tierDefault(TierDark)
}

// TierForBackground classifies a terminal background color ("#rrggbb") into
// the tier whose palettes are designed for it. The boundaries are on CIELAB
// lightness (L*, 0 black — 100 white): below 40 is dark territory, 65 and
// above is light, and the band between gets the softened medium palettes.
func TierForBackground(hex string) (Tier, bool) {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return TierDark, false
	}
	l := lstar(r, g, b)
	switch {
	case l >= 65:
		return TierLight, true
	case l >= 40:
		return TierMedium, true
	default:
		return TierDark, true
	}
}

// tierFromColorFgBg reads the "fg;bg" (rxvt: "fg;default;bg") hint some
// terminals export. Only the background field matters: ANSI 7 or 15 means a
// light background, anything else numeric means dark.
func tierFromColorFgBg(v string) (Tier, bool) {
	parts := strings.Split(v, ";")
	if len(parts) < 2 {
		return TierDark, false
	}
	bg, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
	if err != nil || bg < 0 || bg > 255 {
		return TierDark, false
	}
	if bg == 7 || bg == 15 {
		return TierLight, true
	}
	return TierDark, true
}

// parseHexColor parses "#rrggbb" (case-insensitive, '#' optional).
func parseHexColor(s string) (r, g, b uint8, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}

// lstar converts sRGB to CIELAB lightness L* (0–100). Perceptually uniform,
// unlike raw luminance — mid-gray #808080 lands near 54, not 22.
func lstar(r, g, b uint8) float64 {
	y := 0.2126*srgbToLinear(r) + 0.7152*srgbToLinear(g) + 0.0722*srgbToLinear(b)
	if y <= 216.0/24389.0 {
		return y * 24389.0 / 27.0
	}
	return 116.0*math.Cbrt(y) - 16.0
}

func srgbToLinear(c uint8) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// ── Background contrast adaptation (MUS-36) ─────────────────────────────────
//
// A theme is a foreground palette; the app never paints a full-screen
// background, so body text rides directly on the TERMINAL's own background.
// Tier selection alone is not enough: the dark tier's muted gray was tuned
// against pure black, and a terminal whose background is a dark navy (Ghostty,
// One Dark, Tokyo Night — L* ≈ 10–18, not #000000) collapses its contrast below
// legibility. adaptToBackground closes that gap by nudging the body-text roles
// until each clears a WCAG floor against the background actually detected.

// role floors: the minimum WCAG contrast each body-text role must keep against
// the real terminal background. They descend Fg > FgDim > FgMuted so the
// hierarchy cap below can always satisfy them: FgMuted sits just under the
// muted-gray-on-black contrast the tier-default dark palettes (Nord) ship, so a
// role only moves when it actually falls below its floor on the current terminal
// — a navy is lifted back to that readability, while a palette already clearing
// every floor is returned unchanged. (A muted floor above FgDim's would be
// unreachable under the cap, so muted stays the least-demanding role on every
// tier — on pure white the raw light palettes already clear it and no-op.)
const (
	fgFloor      = 4.5
	fgDimFloor   = 3.0
	fgMutedFloor = 2.7
	// hierarchyGap keeps each role's contrast a visible step below the brighter
	// role's ACHIEVED contrast, so 8-bit rounding can never invert the
	// Fg > FgDim > FgMuted order the eye relies on.
	hierarchyGap = 0.15

	// Selection-highlight (SurfaceBright) floors, dark/medium tiers only.
	// surfaceSepFloor is how much the selected-row block must stand out from the
	// terminal background; selectionTextFloor / selectionDimTextFloor are how
	// legible the Accent / AccentDim selection text must stay on that block. All
	// are set so a near-black terminal (Nord's block sep 2.09, Accent 5.03,
	// AccentDim 2.50) already passes — the block is only re-seated when a
	// mid-tone terminal lets it fade into the surrounding rows (MUS-36). The
	// separation cannot always reach the black baseline (~2.1) because two dark
	// tones can only differ so much, so it targets the most a same-hue block can
	// give while keeping accent text crisp.
	surfaceSepFloor       = 1.5
	selectionTextFloor    = 3.0
	selectionDimTextFloor = 2.3
)

// adaptToBackground returns base with its body-text roles (Fg, FgDim, FgMuted)
// brightened or darkened just enough to clear their WCAG floors against the
// actual terminal background bgHex. It is:
//   - a no-op when bgHex is empty or unparseable (a silent terminal → today's
//     palette, unchanged), and
//   - a per-role no-op when the role already clears its floor, so a near-black
//     terminal keeps the palette byte-for-byte and only genuinely-illegible
//     roles move.
//
// Fg/FgDim/FgMuted (body text) and SurfaceBright (the selected-row highlight)
// are the roles that fade against a mismatched terminal background; those are
// adapted. Surface/Accent/Border and the rest are left as authored — Surface is
// also used as a foreground elsewhere so it must not shift, and for an
// auto-chosen (tier-matched) theme the accents are always legible. For an
// explicit theme deliberately mismatched to the terminal (e.g. a dark theme on a
// white terminal) the unadapted accent/border/progress colors can wash out even
// while the body text stays readable — a known limitation left as a follow-up.
// Runs for both auto and explicit choices; only ever changes a role whose
// contrast has left the [floor, brighter-role − gap] band, so a palette that
// already matches its terminal is returned byte-for-byte unchanged.
func adaptToBackground(base Theme, bgHex string) Theme {
	r, g, b, ok := parseHexColor(bgHex)
	if !ok {
		return base
	}
	bg := colorful.Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255}

	roles := []struct {
		ptr   *lipgloss.Color
		floor float64
	}{
		{&base.Fg, fgFloor},
		{&base.FgDim, fgDimFloor},
		{&base.FgMuted, fgMutedFloor},
	}

	prev := math.Inf(1) // contrast the brighter role actually achieved
	for _, role := range roles {
		fg, err := colorful.Hex(strings.ToLower(string(*role.ptr)))
		if err != nil {
			continue
		}
		// Clamp this role's contrast into [floor, brighter role − gap]. The floor
		// keeps it legible; the ceiling keeps it below the brighter role so the
		// Fg > FgDim > FgMuted order holds. When the palette's own luminance order
		// is inverted for this terminal — e.g. a light theme's muted gray is its
		// LIGHTEST role, which on a dark terminal makes it the highest-contrast —
		// the ceiling pulls the over-bright role back DOWN, not just up. If floor
		// and ceiling cross (a role above can't reach its own floor), the ceiling
		// wins so ordering never inverts.
		natural := wcagContrast(fg, bg)
		want := math.Min(math.Max(natural, role.floor), prev-hierarchyGap)
		var adj colorful.Color
		switch {
		case want > natural+0.001:
			adj = raiseContrast(fg, bg, want)
		case want < natural-0.001:
			adj = lowerContrast(fg, bg, want)
		default:
			adj = fg
		}
		prev = wcagContrast(adj, bg)
		// Only rewrite when the emitted hex actually changed: the no-op path
		// must preserve the original (upper-case) literal so a matching terminal
		// resolves byte-for-byte identically to the raw palette.
		if newHex := adj.Hex(); newHex != strings.ToLower(string(*role.ptr)) {
			*role.ptr = lipgloss.Color(newHex)
		}
	}

	// Selection highlight: SurfaceBright is painted behind accent-colored text as
	// the selected-row block. On a mid-tone terminal it can fade into the
	// surrounding rows; re-seat it so the block stays distinct from the terminal
	// background while accent text on it stays legible. Only the dark/medium
	// tiers use this dark-block-with-bright-accents model — the light tier's
	// selection is a light block with mid-tone accents whose contrasts are
	// deliberately low, so adapting it here would misread the palette as broken
	// and invert it. A light terminal's selection-fade is left as future work.
	if base.Tier != TierLight {
		if sb, err := colorful.Hex(strings.ToLower(string(base.SurfaceBright))); err == nil {
			// The selected row paints Accent (primary) and AccentDim
			// (artist/album) text on the block; both must stay readable wherever
			// the block lands.
			texts := make([]textReq, 0, 2)
			if a, e := colorful.Hex(strings.ToLower(string(base.Accent))); e == nil {
				texts = append(texts, textReq{a, selectionTextFloor})
			}
			if a, e := colorful.Hex(strings.ToLower(string(base.AccentDim))); e == nil {
				texts = append(texts, textReq{a, selectionDimTextFloor})
			}
			adj := fitSelectionSurface(sb, bg, texts, surfaceSepFloor)
			if newHex := adj.Hex(); newHex != strings.ToLower(string(base.SurfaceBright)) {
				base.SurfaceBright = lipgloss.Color(newHex)
			}
		}
	}
	return base
}

// textReq is a color that will be painted on the selection block and the WCAG
// contrast it must keep against it.
type textReq struct {
	col   colorful.Color
	floor float64
}

// fitSelectionSurface re-seats the selection-highlight color sb so the block
// stands out from the terminal background bg (contrast >= sepFloor) while every
// text color painted on it stays legible (contrast >= its floor). It keeps sb's
// hue and walks only its CIELAB lightness, returning the shade CLOSEST to the
// original that satisfies all constraints — so on a near-black/near-white
// terminal, where the palette already passes, it returns sb unchanged. Because
// the accent-text floors rule out lightening the block into the accents' own
// tone, the feasible region on a dark terminal is the darker side, which is also
// where accent text reads best. When nothing can satisfy every constraint (two
// dark tones can only differ so much), it returns the lightness maximizing the
// weakest normalized contrast. Contrast is measured on the emitted 8-bit color.
func fitSelectionSurface(sb, bg colorful.Color, texts []textReq, sepFloor float64) colorful.Color {
	feasible := func(c colorful.Color) (bool, float64) {
		sep := wcagContrast(c, bg)
		ok := sep >= sepFloor
		worst := sep / sepFloor
		for _, t := range texts {
			tc := wcagContrast(t.col, c)
			if tc < t.floor {
				ok = false
			}
			if r := tc / t.floor; r < worst {
				worst = r
			}
		}
		return ok, worst
	}
	if ok, _ := feasible(sb); ok {
		return sb
	}
	origL, a, b := sb.Lab()
	bestFeasibleL, bestFeasibleDist := -1.0, math.Inf(1)
	bestScoreL, bestScore := origL, math.Inf(-1)
	const steps = 256
	for i := 0; i <= steps; i++ {
		l := float64(i) / steps
		c := colorAtL(l, a, b)
		ok, worst := feasible(c)
		if worst > bestScore {
			bestScore, bestScoreL = worst, l
		}
		if ok {
			if d := math.Abs(l - origL); d < bestFeasibleDist {
				bestFeasibleDist, bestFeasibleL = d, l
			}
		}
	}
	if bestFeasibleL >= 0 {
		return colorAtL(bestFeasibleL, a, b)
	}
	return colorAtL(bestScoreL, a, b)
}

// raiseContrast returns fg unchanged if it already meets target contrast against
// bg; otherwise it walks fg's CIELAB lightness (keeping hue and chroma) toward
// whichever pole — white or black — gains contrast fastest, and returns the
// nearest shade that reaches target. Contrast is measured on the 8-bit color
// actually emitted (colorAtL round-trips through hex), so the result provably
// clears target after rounding. If even the pole cannot reach target (the
// background sits too close to this hue's luminance, e.g. a mid-gray terminal),
// it returns pure white or black — the maximum achievable separation.
func raiseContrast(fg, bg colorful.Color, target float64) colorful.Color {
	if wcagContrast(fg, bg) >= target {
		return fg
	}
	l, a, bb := fg.Lab()
	lighten := wcagContrast(colorAtL(1, a, bb), bg) >= wcagContrast(colorAtL(0, a, bb), bg)
	lo, hi := l, l
	if lighten {
		hi = 1
	} else {
		lo = 0
	}
	for i := 0; i < 32; i++ {
		mid := (lo + hi) / 2
		if wcagContrast(colorAtL(mid, a, bb), bg) >= target {
			if lighten {
				hi = mid
			} else {
				lo = mid
			}
		} else {
			if lighten {
				lo = mid
			} else {
				hi = mid
			}
		}
	}
	edge := hi
	if !lighten {
		edge = lo
	}
	if cand := colorAtL(edge, a, bb); wcagContrast(cand, bg) >= target {
		return cand
	}
	if lighten {
		return colorful.Color{R: 1, G: 1, B: 1}
	}
	return colorful.Color{R: 0, G: 0, B: 0}
}

// lowerContrast moves fg toward the background's own lightness (keeping hue)
// until its contrast against bg drops to about target — the opposite of
// raiseContrast. It is how a dimmer role is pulled back UNDER a brighter one
// when the palette's luminance order is inverted for this terminal (a light
// theme's muted gray, its lightest role, would otherwise out-contrast FgDim on a
// dark terminal). Contrast is measured on the emitted 8-bit color.
func lowerContrast(fg, bg colorful.Color, target float64) colorful.Color {
	l, a, b := fg.Lab()
	bl, _, _ := bg.Lab()
	// Contrast is monotonic from fg's lightness (high) toward bg's (→1); binary
	// search for the lightness nearest fg that still clears target.
	lo, hi := l, bl
	for i := 0; i < 32; i++ {
		mid := (lo + hi) / 2
		if wcagContrast(colorAtL(mid, a, b), bg) >= target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return colorAtL(lo, a, b)
}

// colorAtL sets CIELAB lightness to l (0–1, keeping a and b), clamps into the
// sRGB gamut, and round-trips through the 8-bit hex the theme will emit — so
// callers measure the color as actually rendered, not an unrepresentable ideal.
func colorAtL(l, a, b float64) colorful.Color {
	c := colorful.Lab(l, a, b).Clamped()
	if q, err := colorful.Hex(c.Hex()); err == nil {
		return q
	}
	return c
}

// wcagContrast is the WCAG 2.x contrast ratio (L1+0.05)/(L2+0.05) between two
// colors, mirroring the sRGB luminance math the theme (and the test oracle) use
// so adaptToBackground targets exactly the ratio the tests measure.
func wcagContrast(x, y colorful.Color) float64 {
	lx, ly := relLuminance(x), relLuminance(y)
	if lx < ly {
		lx, ly = ly, lx
	}
	return (lx + 0.05) / (ly + 0.05)
}

func relLuminance(c colorful.Color) float64 {
	lin := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// GradientProgress renders a progress bar with a gradient fill effect.
// filled and total are character counts. The filled portion transitions
// across 3 color zones: AccentDim → Accent → BorderFocused for a
// visible gradient ramp.
func (t Theme) GradientProgress(filled, total int) string {
	if total <= 0 {
		return ""
	}
	if filled > total {
		filled = total
	}
	empty := total - filled

	var b strings.Builder

	// Use block characters for a smoother, more "glossy" fill
	zone1 := filled / 3
	zone2 := filled / 3
	zone3 := filled - zone1 - zone2

	if zone1 > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.AccentDim).Render(strings.Repeat("━", zone1)))
	}
	if zone2 > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.Accent).Render(strings.Repeat("━", zone2)))
	}
	if zone3 > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.BorderFocused).Render(strings.Repeat("━", zone3)))
	}

	// Bright dot at current position
	if filled > 0 && filled < total {
		b.WriteString(lipgloss.NewStyle().Foreground(t.BorderFocused).Bold(true).Render("●"))
		empty--
	}

	if empty > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.ProgressBg).Render(strings.Repeat("─", empty)))
	}

	return b.String()
}
