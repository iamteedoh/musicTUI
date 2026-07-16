package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type settingItem struct {
	Key   string
	Label string
	// Value renders the setting's current state. th is the active resolved
	// theme, so the Theme row can show what Auto actually picked.
	Value func(config.Config, theme.Theme) string
}

var settingItems = []settingItem{
	{"theme", "Theme", func(c config.Config, th theme.Theme) string {
		if c.Theme == "" || c.Theme == theme.Auto {
			// Auto shows its answer, so a user on a light terminal can see
			// the detection worked before deciding to override it.
			return "Auto — " + th.Name
		}
		t := theme.FromName(c.Theme)
		return t.Name + " (" + string(t.Tier) + ")"
	}},
	{"check_duplicates", "Check for duplicate playlists on startup", func(c config.Config, _ theme.Theme) string {
		if c.CheckDuplicates {
			return "On"
		}
		return "Off"
	}},
}

type Settings struct {
	Selected int
}

func NewSettings() Settings {
	return Settings{}
}

func (s *Settings) Up() {
	if s.Selected > 0 {
		s.Selected--
	}
}

func (s *Settings) Down() {
	if s.Selected < len(settingItems)-1 {
		s.Selected++
	}
}

func (s Settings) SelectedKey() string {
	if s.Selected < len(settingItems) {
		return settingItems[s.Selected].Key
	}
	return ""
}

func (s Settings) View(th theme.Theme, cfg config.Config, width, height int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("✦  Settings")
	b.WriteString(" " + title + "\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n\n")

	for i, item := range settingItems {
		label := item.Label
		value := item.Value(cfg, th)

		var valueStyle lipgloss.Style
		switch value {
		case "On":
			valueStyle = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
		case "Off":
			valueStyle = lipgloss.NewStyle().Foreground(th.FgMuted)
		default:
			// Multi-choice values (the theme name) read as content, not state.
			valueStyle = lipgloss.NewStyle().Foreground(th.FgDim)
		}

		if i == s.Selected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸ ")
			labelStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(label)
			valStr := valueStyle.Render(fmt.Sprintf(" [%s]", value))
			line := lipgloss.NewStyle().
				Width(width).
				Background(th.SurfaceBright).
				Render(indicator + labelStr + valStr)
			b.WriteString(line + "\n")
		} else {
			labelStr := lipgloss.NewStyle().Foreground(th.Fg).Render(label)
			valStr := valueStyle.Render(fmt.Sprintf(" [%s]", value))
			b.WriteString("  " + labelStr + valStr + "\n")
		}
	}

	b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Enter:change  ←/→:cycle  Esc:back"))

	return b.String()
}
