package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// Onboard is a first-run walkthrough that explains the Spotify developer-app
// setup without requiring the user to leave the TUI to read docs. It is its
// own full-screen view while Active is true.
type Onboard struct {
	Active         bool
	Step           int
	ClientIDInput  string
	CursorPos      int
	Error          string
}

// TotalSteps is the number of steps in the wizard (including the final
// "submit client_id" step). Kept as a method so the renderer can show a
// progress dot row without the caller passing it around.
const TotalSteps = 5

func NewOnboard() Onboard {
	return Onboard{}
}

// Start puts the wizard in its initial state. Call on first launch when
// no client_id is configured.
func (o *Onboard) Start() {
	o.Active = true
	o.Step = 0
	o.ClientIDInput = ""
	o.CursorPos = 0
	o.Error = ""
}

func (o *Onboard) Close() {
	o.Active = false
}

func (o *Onboard) Next() {
	if o.Step < TotalSteps-1 {
		o.Step++
	}
}

func (o *Onboard) Prev() {
	if o.Step > 0 {
		o.Step--
	}
}

func (o *Onboard) InputChar(r rune) {
	if o.Step != TotalSteps-1 {
		return
	}
	before := o.ClientIDInput[:o.CursorPos]
	after := o.ClientIDInput[o.CursorPos:]
	o.ClientIDInput = before + string(r) + after
	o.CursorPos += len(string(r))
}

func (o *Onboard) Backspace() {
	if o.Step != TotalSteps-1 || o.CursorPos == 0 {
		return
	}
	runes := []rune(o.ClientIDInput)
	runePos := len([]rune(o.ClientIDInput[:o.CursorPos]))
	if runePos == 0 {
		return
	}
	newRunes := append(runes[:runePos-1], runes[runePos:]...)
	o.ClientIDInput = string(newRunes)
	o.CursorPos = len(string(newRunes[:runePos-1]))
}

// OnFinalStep reports whether we're on the final "paste Client ID" step
// so callers know Enter should submit rather than advance.
func (o Onboard) OnFinalStep() bool {
	return o.Step == TotalSteps-1
}

// ClientID returns the trimmed client ID the user has typed so far.
func (o Onboard) ClientID() string {
	return strings.TrimSpace(o.ClientIDInput)
}

// ─────────────────────────── Rendering ───────────────────────────

func (o Onboard) View(th theme.Theme, width, height int) string {
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
		Render("Welcome to musicTUI — Spotify Setup")

	var body string
	switch o.Step {
	case 0:
		body = o.viewWelcome(th)
	case 1:
		body = o.viewOpenDashboard(th)
	case 2:
		body = o.viewCreateApp(th)
	case 3:
		body = o.viewAddUser(th)
	case 4:
		body = o.viewPasteClientID(th)
	}

	dots := o.progressDots(th)
	footer := o.footer(th)

	content := title + "\n" +
		lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", 60)) + "\n\n" +
		body + "\n\n" +
		dots + "\n" +
		footer

	if o.Error != "" {
		content += "\n\n" + lipgloss.NewStyle().Foreground(th.Error).Render("⚠ "+o.Error)
	}

	boxed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Accent).
		Padding(1, 2).
		Width(66).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, boxed)
}

func (o Onboard) progressDots(th theme.Theme) string {
	activeStyle := lipgloss.NewStyle().Foreground(th.Accent)
	dimStyle := lipgloss.NewStyle().Foreground(th.FgDim)
	var parts []string
	for i := 0; i < TotalSteps; i++ {
		if i == o.Step {
			parts = append(parts, activeStyle.Render("●"))
		} else {
			parts = append(parts, dimStyle.Render("○"))
		}
	}
	return lipgloss.NewStyle().Align(lipgloss.Center).Width(60).
		Render(strings.Join(parts, " "))
}

func (o Onboard) footer(th theme.Theme) string {
	key := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgMuted)
	sep := dim.Render("  ·  ")
	var hints []string
	if o.Step > 0 {
		hints = append(hints, key.Render("←")+dim.Render(": back"))
	}
	if o.OnFinalStep() {
		hints = append(hints, key.Render("Enter")+dim.Render(": finish"))
	} else {
		hints = append(hints, key.Render("→ / Enter")+dim.Render(": next"))
	}
	if o.Step == 1 {
		hints = append(hints, key.Render("O")+dim.Render(": open dashboard"))
	}
	hints = append(hints, key.Render("Esc")+dim.Render(": skip"))
	return lipgloss.NewStyle().Align(lipgloss.Center).Width(60).
		Render(strings.Join(hints, sep))
}

func (o Onboard) viewWelcome(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(60)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(60)
	var b strings.Builder
	b.WriteString(body.Render("musicTUI streams your own Spotify account. Before we connect, Spotify requires a free one-time setup that takes about 3 minutes."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("We'll walk you through it right here — no need to leave this terminal."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("You'll need: a Spotify account (Free or Premium) and a web browser."))
	return b.String()
}

func (o Onboard) viewOpenDashboard(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(60)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(60)
	var b strings.Builder
	b.WriteString(body.Render("First, we'll open the Spotify Developer Dashboard in your browser."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("URL (if the browser doesn't open automatically):"))
	b.WriteString("\n")
	b.WriteString(accent.Render("https://developer.spotify.com/dashboard"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Log in with your Spotify account when prompted, then press → to continue."))
	return b.String()
}

func (o Onboard) viewCreateApp(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(60)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	label := lipgloss.NewStyle().Foreground(th.FgMuted)
	var b strings.Builder
	b.WriteString(body.Render("On the dashboard, click \"Create App\" and fill in:"))
	b.WriteString("\n\n")
	b.WriteString(label.Render("  App name:       ") + accent.Render("musicTUI") + "\n")
	b.WriteString(label.Render("  Description:    ") + accent.Render("Terminal music player") + "\n")
	b.WriteString(label.Render("  Redirect URI:   ") + accent.Render("http://127.0.0.1:8888/callback") + "\n")
	b.WriteString(label.Render("  Which APIs:     ") + accent.Render("☑ Web API") + "\n")
	b.WriteString("\n")
	b.WriteString(body.Render("Click Save, then press → to continue."))
	return b.String()
}

func (o Onboard) viewAddUser(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(60)
	warn := lipgloss.NewStyle().Foreground(th.Warning).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(54)

	// Boxed warning to visually separate this from the other steps —
	// this is the #1 onboarding trap and skipping it silently breaks
	// most features later.
	warnBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Warning).
		Padding(0, 1).
		Width(58).
		Render(
			warn.Render("⚠  Important — don't skip this step") + "\n\n" +
				muted.Render("Without adding your account under User Management, login will succeed but these will all fail silently:") + "\n" +
				muted.Render("  · browsing tracks inside your playlists") + "\n" +
				muted.Render("  · adding songs to playlists") + "\n" +
				muted.Render("  · seeing recent listens or your saved library"),
		)

	var b strings.Builder
	b.WriteString(warnBox)
	b.WriteString("\n\n")
	b.WriteString(body.Render("On your new app page in the Spotify dashboard, click the \"User Management\" tab, then \"Add User\". Enter the email address linked to your Spotify account and save."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("When done, press → to continue."))
	return b.String()
}

func (o Onboard) viewPasteClientID(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(60)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(60)
	var b strings.Builder
	b.WriteString(body.Render("Almost there. On your app page, copy the Client ID (a long string of letters and numbers) and paste it below."))
	b.WriteString("\n\n")

	// Input field with cursor
	val := o.ClientIDInput
	if o.CursorPos > len(val) {
		o.CursorPos = len(val)
	}
	before := val[:o.CursorPos]
	after := val[o.CursorPos:]
	cursor := lipgloss.NewStyle().Background(th.Accent).Render(" ")
	display := before + cursor + after
	if val == "" {
		display = cursor
	}
	field := lipgloss.NewStyle().
		Foreground(th.Fg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Accent).
		Width(58).
		Padding(0, 1).
		Render(display)
	b.WriteString(field)
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press Enter to save and log in to Spotify."))
	return b.String()
}
