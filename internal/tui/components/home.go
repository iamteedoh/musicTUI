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

	// AppOwnerNotPremium is set when Spotify rejects API calls because the
	// Developer app behind the configured client_id is owned by a non-Premium
	// account. Triggers an actionable recovery block instead of the normal
	// "not connected" prompt.
	AppOwnerNotPremium bool
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
	if h.AppOwnerNotPremium {
		// Login succeeded but Spotify is 403-ing every API call because the
		// Developer app's owner account isn't Premium. Re-auth can't fix this,
		// so spell out the recovery steps.
		errStyle := lipgloss.NewStyle().Foreground(th.Error).Bold(true)
		lines = append(lines,
			accent.Bold(true).Render("musicTUI"),
			errStyle.Render("✗ Spotify blocked this app"),
			"",
			dim.Render("Login worked, but Spotify rejects every request because the"),
			dim.Render("Developer app behind your Client ID is owned by an account"),
			dim.Render("without active Premium. (It checks the app OWNER, not you.)"),
			"",
			muted.Render("How to fix:"),
			dim.Render("1. Open https://developer.spotify.com/dashboard"),
			dim.Render("   signed in as a Premium account."),
			dim.Render("2. Delete this app, then create a new one."),
			dim.Render("3. Set Redirect URI to http://127.0.0.1:8888/callback"),
			dim.Render("4. Press Ctrl+O and paste the new Client ID."),
			"",
			muted.Render("Note: after a subscription change Spotify can take a few"),
			muted.Render("hours to allow requests again."),
		)
	} else if h.Username != "" {
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
			"",
			muted.Render(`Browser says "Invalid client id"? Your Spotify Client`),
			muted.Render("ID is wrong — press Ctrl+O to re-enter it."),
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
