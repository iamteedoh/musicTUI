package theme

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// Resolve turns a configured theme name into a Theme. Explicit names win.
// "auto" (or empty) matches the terminal instead: termBg is the terminal's
// reported background color ("#rrggbb" from the termcap probe, empty when it
// didn't answer), with the COLORFGBG environment variable as a coarse
// fallback for terminals the probe can't reach (e.g. under tmux). No signal
// at all means dark — the assumption the app has always made.
func Resolve(name, termBg string) Theme {
	key := strings.ToLower(strings.TrimSpace(name))
	if key != "" && key != Auto {
		return FromName(name)
	}
	if t, ok := TierForBackground(termBg); ok {
		return tierDefault(t)
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
