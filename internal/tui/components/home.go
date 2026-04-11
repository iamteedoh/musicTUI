package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musictui-go/internal/theme"
)

type Home struct {
	Username string
}

func NewHome() Home {
	return Home{}
}

func (h Home) View(th theme.Theme, width, height int) string {
	accent := lipgloss.NewStyle().Foreground(th.Accent)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)

	var lines []string

	// Greeting + auth status
	if h.Username != "" {
		lines = append(lines,
			accent.Bold(true).Render("Welcome, "+h.Username),
			lipgloss.NewStyle().Foreground(th.Success).Render("● Connected to Spotify"),
			lipgloss.NewStyle().Foreground(th.Accent).Render("● Authenticated as "+h.Username),
		)
	} else {
		lines = append(lines,
			accent.Bold(true).Render("musicTUI"),
			lipgloss.NewStyle().Foreground(th.Warning).Render("○ Not connected — press Ctrl+L"),
		)
	}

	lines = append(lines, "")
	lines = append(lines, muted.Render("KEYBOARD SHORTCUTS"))
	lines = append(lines, muted.Render(strings.Repeat("─", 52)))
	lines = append(lines, "")

	// Two-column keybindings
	keyStyle := accent.Bold(true).Width(8).Align(lipgloss.Right)
	descStyle := dim.PaddingLeft(1).Width(17)

	type kb struct{ key, desc string }
	left := []kb{
		{"j / k", "Navigate"},
		{"Enter", "Select / Play"},
		{"/", "Quick search"},
		{"Tab", "Switch focus"},
		{"Esc", "Go back"},
	}
	right := []kb{
		{"Space", "Play / Pause"},
		{"+ / -", "Volume"},
		{"n / p", "Next / Prev"},
		{"s", "Shuffle"},
		{"l", "Lyrics"},
	}

	for i := 0; i < len(left); i++ {
		l := keyStyle.Render(left[i].key) + descStyle.Render(left[i].desc)
		r := keyStyle.Render(right[i].key) + descStyle.Render(right[i].desc)
		lines = append(lines, l+"  "+r)
	}

	lines = append(lines, "")
	lines = append(lines, muted.Italic(true).Render("q to quit"))

	// Build the content block
	content := strings.Join(lines, "\n")

	// Center vertically
	contentH := len(lines)
	topPad := (height - contentH) / 2
	if topPad < 0 {
		topPad = 0
	}

	centered := lipgloss.NewStyle().
		Width(width).
		Align(lipgloss.Center).
		Render(content)

	return strings.Repeat("\n", topPad) + centered
}
