package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musictui-go/internal/config"
	"github.com/iamteedoh/musictui-go/internal/theme"
)

type settingItem struct {
	Key   string
	Label string
	Value func(config.Config) string
}

var settingItems = []settingItem{
	{"check_duplicates", "Check for duplicate playlists on startup", func(c config.Config) string {
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
		value := item.Value(cfg)

		var valueStyle lipgloss.Style
		if value == "On" {
			valueStyle = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
		} else {
			valueStyle = lipgloss.NewStyle().Foreground(th.FgMuted)
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

	b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Enter:toggle  Esc:back"))

	return b.String()
}
