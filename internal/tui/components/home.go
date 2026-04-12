package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type Home struct {
	Username        string
	AuthURL         string // shown when waiting for browser auth
	NeedsConfig     bool   // true when no Spotify client_id is configured
	Version         string // current build version ("dev" for local builds)
	UpdateAvailable string // latest release tag if newer than Version; empty otherwise
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
	} else if h.AuthURL != "" {
		lines = append(lines,
			accent.Bold(true).Render("musicTUI"),
			lipgloss.NewStyle().Foreground(th.Warning).Render("○ Waiting for login in browser..."),
			"",
			muted.Render("If the browser didn't open, visit:"),
			dim.Render(h.AuthURL),
		)
	} else {
		prompt := "○ Not connected — press Ctrl+L to log in"
		if h.NeedsConfig {
			prompt = "○ Not set up — press Ctrl+L to enter your Spotify Client ID"
		}
		lines = append(lines,
			accent.Bold(true).Render("musicTUI"),
			lipgloss.NewStyle().Foreground(th.Warning).Render(prompt),
		)
		if h.NeedsConfig {
			lines = append(lines,
				"",
				muted.Render("Get a free Client ID at:"),
				dim.Render("https://developer.spotify.com/dashboard"),
			)
		}
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

	// Version + update banner
	if h.Version != "" {
		versionLine := muted.Render(h.Version)
		if h.UpdateAvailable != "" {
			updateStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
			versionLine = muted.Render(h.Version+"  •  ") +
				updateStyle.Render("⬆ "+h.UpdateAvailable+" available — press Ctrl+U to update")
		}
		lines = append(lines, "")
		lines = append(lines, versionLine)
	}

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
