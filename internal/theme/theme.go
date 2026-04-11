package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type Theme struct {
	Name           string
	BaseBg         lipgloss.Color // Darkest background — outer frame and panel gaps
	Bg             lipgloss.Color
	Fg             lipgloss.Color
	FgDim          lipgloss.Color
	FgMuted        lipgloss.Color
	Surface        lipgloss.Color
	SurfaceBright  lipgloss.Color
	Accent         lipgloss.Color
	AccentDim      lipgloss.Color
	Success        lipgloss.Color
	Warning        lipgloss.Color
	Error          lipgloss.Color
	SidebarBg      lipgloss.Color
	SidebarSel     lipgloss.Color
	NowPlayingBg   lipgloss.Color
	Border         lipgloss.Color
	BorderFocused  lipgloss.Color
	ProgressBar    lipgloss.Color
	ProgressBg     lipgloss.Color
}

func Nord() Theme {
	return Theme{
		Name:          "Nord",
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
		Name:          "Catppuccin",
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
		Name:          "Gruvbox",
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

var AllThemes = []string{"nord", "dracula", "catppuccin", "gruvbox", "tokyo_night"}

func FromName(name string) Theme {
	switch name {
	case "dracula":
		return Dracula()
	case "catppuccin":
		return Catppuccin()
	case "gruvbox":
		return Gruvbox()
	case "tokyo_night", "tokyo-night", "tokyonight":
		return TokyoNight()
	default:
		return Nord()
	}
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
