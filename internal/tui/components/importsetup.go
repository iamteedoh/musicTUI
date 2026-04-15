package components

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// ImportSetup is a full-screen, multi-step wizard for the one-time
// OAuth credential setup the embedded import feature needs. It
// walks the user through:
//
//  1. Welcome / overview
//  2. Google Cloud — create project
//  3. Google Cloud — enable YouTube Data API v3
//  4. Google Cloud — OAuth consent screen + add yourself as test user
//  5. Google Cloud — create OAuth Desktop client
//  6. Paste Google client_id + client_secret
//  7. Spotify — add the import redirect URI to your existing app
//  8. Paste Spotify client_secret
//  9. Done
//
// Mirrors the existing Onboard component's structure but with two
// text-input steps (each collecting two adjacent fields) instead
// of one. Active=true means the wizard takes over the entire
// screen and input.
type ImportSetup struct {
	Active bool
	Step   int
	Error  string

	// Step 6 fields
	GoogleClientID     string
	GoogleClientSecret string

	// Step 8 field
	SpotifyClientSecret string

	// Which of the two fields the cursor is on (0 = top, 1 = bottom).
	// Used by steps 6 (two fields) and 8 (one field — Field always 0).
	Field int
	// CursorPos is a RUNE index into the active field. Rune units (not
	// bytes) so the cursor math stays correct when pasted content
	// contains multi-byte UTF-8, and so the mask rendering (which
	// substitutes one `•` per rune) aligns with the cursor.
	CursorPos int
}

const ImportSetupTotalSteps = 9

// NewImportSetup returns an inactive wizard. Call Start() to launch.
func NewImportSetup() ImportSetup { return ImportSetup{} }

// Start (re)initializes the wizard state, optionally pre-filled
// from existing config so the user can edit a partially-completed
// setup without retyping.
func (w *ImportSetup) Start(googleID, googleSecret, spotifySecret string) {
	*w = ImportSetup{
		Active:              true,
		Step:                0,
		GoogleClientID:      googleID,
		GoogleClientSecret:  googleSecret,
		SpotifyClientSecret: spotifySecret,
	}
}

func (w *ImportSetup) Close() { w.Active = false }

func (w *ImportSetup) Next() {
	if w.Step < ImportSetupTotalSteps-1 {
		w.Step++
		w.Field = 0
		w.CursorPos = 0
	}
}

func (w *ImportSetup) Prev() {
	if w.Step > 0 {
		w.Step--
		w.Field = 0
		w.CursorPos = 0
	}
}

// IsInputStep returns true on the steps that take typed input
// rather than just navigation.
func (w ImportSetup) IsInputStep() bool {
	return w.Step == 5 || w.Step == 7
}

// IsFinalStep returns true on the very last (Done) screen.
func (w ImportSetup) IsFinalStep() bool {
	return w.Step == ImportSetupTotalSteps-1
}

// activeField returns a pointer to the string the cursor is editing.
// Nil on non-input steps so callers can no-op.
func (w *ImportSetup) activeField() *string {
	switch w.Step {
	case 5:
		if w.Field == 0 {
			return &w.GoogleClientID
		}
		return &w.GoogleClientSecret
	case 7:
		return &w.SpotifyClientSecret
	}
	return nil
}

// FieldsOnCurrentStep returns how many editable fields the current
// step has (so SwitchField knows whether Tab should advance or wrap).
func (w ImportSetup) FieldsOnCurrentStep() int {
	switch w.Step {
	case 5:
		return 2
	case 7:
		return 1
	}
	return 0
}

// SwitchField cycles the cursor between fields on multi-field steps.
func (w *ImportSetup) SwitchField() {
	n := w.FieldsOnCurrentStep()
	if n <= 1 {
		return
	}
	w.Field = (w.Field + 1) % n
	if af := w.activeField(); af != nil {
		w.CursorPos = utf8.RuneCountInString(*af)
	}
}

// InputChar inserts a character at the cursor on input steps. All
// string math is rune-based: CursorPos counts runes, not bytes, so
// multi-byte UTF-8 pastes insert cleanly.
func (w *ImportSetup) InputChar(r rune) {
	if !w.IsInputStep() {
		return
	}
	field := w.activeField()
	if field == nil {
		return
	}
	rs := []rune(*field)
	if w.CursorPos > len(rs) {
		w.CursorPos = len(rs)
	}
	rs = append(rs[:w.CursorPos], append([]rune{r}, rs[w.CursorPos:]...)...)
	*field = string(rs)
	w.CursorPos++
}

// Backspace removes the rune left of the cursor.
func (w *ImportSetup) Backspace() {
	if !w.IsInputStep() {
		return
	}
	field := w.activeField()
	if field == nil || w.CursorPos == 0 {
		return
	}
	rs := []rune(*field)
	if w.CursorPos > len(rs) {
		w.CursorPos = len(rs)
	}
	rs = append(rs[:w.CursorPos-1], rs[w.CursorPos:]...)
	*field = string(rs)
	w.CursorPos--
}

// Paste handles a paste operation by inserting `s` at the cursor.
// Strips control chars first to keep pasted blobs clean.
func (w *ImportSetup) Paste(s string) {
	if !w.IsInputStep() {
		return
	}
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	for _, r := range clean {
		w.InputChar(r)
	}
}

// Trimmed returns the non-whitespace credential values.
func (w ImportSetup) Trimmed() (gID, gSecret, sSecret string) {
	return strings.TrimSpace(w.GoogleClientID),
		strings.TrimSpace(w.GoogleClientSecret),
		strings.TrimSpace(w.SpotifyClientSecret)
}

// Complete reports whether all three credential fields are non-empty.
func (w ImportSetup) Complete() bool {
	g, gs, ss := w.Trimmed()
	return g != "" && gs != "" && ss != ""
}

// URLForStep returns the recommended URL for the current step's
// "Open in browser" key (O), or "" if no URL applies.
func (w ImportSetup) URLForStep() string {
	switch w.Step {
	case 1:
		return "https://console.cloud.google.com/projectcreate"
	case 2:
		return "https://console.cloud.google.com/apis/library/youtube.googleapis.com"
	case 3:
		return "https://console.cloud.google.com/auth/branding"
	case 4:
		return "https://console.cloud.google.com/apis/credentials"
	case 6:
		return "https://developer.spotify.com/dashboard"
	}
	return ""
}

// ─────────────────────────── Rendering ───────────────────────────

func (w ImportSetup) View(th theme.Theme, width, height int) string {
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
		Render("musicTUI Import — One-Time Setup")

	var body string
	switch w.Step {
	case 0:
		body = w.viewWelcome(th)
	case 1:
		body = w.viewGoogleCreateProject(th)
	case 2:
		body = w.viewGoogleEnableAPI(th)
	case 3:
		body = w.viewGoogleConsent(th)
	case 4:
		body = w.viewGoogleCreateOAuth(th)
	case 5:
		body = w.viewGooglePasteCreds(th)
	case 6:
		body = w.viewSpotifyRedirect(th)
	case 7:
		body = w.viewSpotifyPasteSecret(th)
	case 8:
		body = w.viewDone(th)
	}

	dots := w.progressDots(th)
	footer := w.footer(th)

	content := title + "\n" +
		lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", 64)) + "\n\n" +
		body + "\n\n" +
		dots + "\n" +
		footer

	if w.Error != "" {
		content += "\n\n" + lipgloss.NewStyle().Foreground(th.Error).Render("⚠ "+w.Error)
	}

	boxed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Accent).
		Padding(1, 2).
		Width(70).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, boxed)
}

func (w ImportSetup) progressDots(th theme.Theme) string {
	active := lipgloss.NewStyle().Foreground(th.Accent)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)
	var parts []string
	for i := 0; i < ImportSetupTotalSteps; i++ {
		if i == w.Step {
			parts = append(parts, active.Render("●"))
		} else {
			parts = append(parts, dim.Render("○"))
		}
	}
	return lipgloss.NewStyle().Align(lipgloss.Center).Width(64).
		Render(strings.Join(parts, " "))
}

func (w ImportSetup) footer(th theme.Theme) string {
	key := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgMuted)
	sep := dim.Render("  ·  ")
	var hints []string
	if w.Step > 0 {
		hints = append(hints, key.Render("←")+dim.Render(": back"))
	}
	if w.IsInputStep() {
		if w.FieldsOnCurrentStep() > 1 {
			hints = append(hints, key.Render("Tab")+dim.Render(": switch field"))
		}
		hints = append(hints, key.Render("Enter")+dim.Render(": next"))
	} else if w.IsFinalStep() {
		hints = append(hints, key.Render("Enter")+dim.Render(": save & finish"))
	} else {
		hints = append(hints, key.Render("→ / Enter")+dim.Render(": next"))
	}
	if w.URLForStep() != "" {
		hints = append(hints, key.Render("O")+dim.Render(": open in browser"))
	}
	hints = append(hints, key.Render("Esc")+dim.Render(": cancel"))
	return lipgloss.NewStyle().Align(lipgloss.Center).Width(64).
		Render(strings.Join(hints, sep))
}

// ─────────────────── per-step views ───────────────────

func (w ImportSetup) viewWelcome(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(body.Render("This wizard sets up the library-import feature. It transfers your YouTube Music playlists into Spotify (more services later)."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Because we don't run a server, you create your own free OAuth apps with Google and Spotify. That means:"))
	b.WriteString("\n\n")
	b.WriteString("  " + accent.Render("✓") + "  Your tokens never leave your machine\n")
	b.WriteString("  " + accent.Render("✓") + "  You get your own API quota (not shared)\n")
	b.WriteString("  " + accent.Render("✓") + "  No verification required (you're the only user)\n")
	b.WriteString("\n")
	b.WriteString(muted.Render("Total time: about 10 minutes. You can cancel anytime; partial progress is preserved."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("You'll need: a Google account, your Spotify dev-app dashboard access, and a web browser."))
	return b.String()
}

func (w ImportSetup) viewGoogleCreateProject(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Step 1 of 4 — Google Cloud") + "\n")
	b.WriteString(body.Render("Create a Google Cloud project to host your YouTube OAuth app."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press O to open:"))
	b.WriteString("\n  " + accent.Render(w.URLForStep()))
	b.WriteString("\n\n")
	b.WriteString(body.Render("On the form:"))
	b.WriteString("\n  • " + body.Render("Project name: anything (e.g. \"musictui-import\")"))
	b.WriteString("\n  • " + body.Render("Organisation: leave default"))
	b.WriteString("\n  • " + body.Render("Click Create"))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Wait for the \"Project created\" notification, then make sure your new project is selected in the top bar before continuing."))
	return b.String()
}

func (w ImportSetup) viewGoogleEnableAPI(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Step 2 of 4 — Enable YouTube Data API v3") + "\n")
	b.WriteString(body.Render("This is the API the import uses to read your YT Music library."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press O to open:"))
	b.WriteString("\n  " + accent.Render(w.URLForStep()))
	b.WriteString("\n\n")
	b.WriteString(body.Render("On the page:"))
	b.WriteString("\n  • " + body.Render("Confirm your project is selected (top bar)"))
	b.WriteString("\n  • " + body.Render("Click the blue Enable button"))
	b.WriteString("\n  • " + body.Render("Wait for activation to confirm"))
	return b.String()
}

func (w ImportSetup) viewGoogleConsent(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	warn := lipgloss.NewStyle().Foreground(th.Warning).Bold(true)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Step 3 of 4 — OAuth Consent Screen") + "\n")
	b.WriteString(body.Render("Tells Google your app exists and who can use it."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press O to open:"))
	b.WriteString("\n  " + accent.Render(w.URLForStep()))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Click the \"Get started\" button, then walk through"))
	b.WriteString("\n" + body.Render("Google's 4-step mini-wizard:"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("  1. App Information:"))
	b.WriteString("\n     • App name: musicTUI Import")
	b.WriteString("\n     • User support email: your Gmail")
	b.WriteString("\n     → Next")
	b.WriteString("\n  2. Audience:")
	b.WriteString("\n     • Select External")
	b.WriteString("\n     → Next")
	b.WriteString("\n  3. Contact Information:")
	b.WriteString("\n     • Email addresses: your Gmail")
	b.WriteString("\n     → Next")
	b.WriteString("\n  4. Finish:")
	b.WriteString("\n     • ☑ Agree to Google API Services User Data Policy")
	b.WriteString("\n     → Continue → Create")
	b.WriteString("\n\n")
	b.WriteString(warn.Render("⚠ After creation, click the \"Audience\" tab and add"))
	b.WriteString("\n" + warn.Render("  your Gmail under \"Test users\" — or sign-in will"))
	b.WriteString("\n" + warn.Render("  be blocked with an \"Access blocked\" error."))
	_ = muted
	return b.String()
}

func (w ImportSetup) viewGoogleCreateOAuth(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Step 4 of 4 — Create OAuth Credentials") + "\n")
	b.WriteString(body.Render("This is what musicTUI uses to identify itself to Google."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press O to open:"))
	b.WriteString("\n  " + accent.Render(w.URLForStep()))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Click Create Credentials → OAuth client ID:"))
	b.WriteString("\n  • " + body.Render("Application type: Web application"))
	b.WriteString("\n  • " + body.Render("Name: musicTUI Import"))
	b.WriteString("\n  • " + body.Render("Authorized redirect URI:"))
	b.WriteString("\n      " + accent.Render("http://127.0.0.1:8889/callback"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Click Create. A popup shows your Client ID and Client Secret — keep it open for the next step."))
	return b.String()
}

func (w ImportSetup) viewGooglePasteCreds(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Paste Google OAuth Credentials") + "\n")
	b.WriteString(body.Render("Copy from the popup in your browser, then paste below."))
	b.WriteString("\n\n")
	b.WriteString(w.renderField("Client ID", w.GoogleClientID, w.Field == 0, false, th))
	b.WriteString("\n")
	b.WriteString(w.renderField("Client Secret", w.GoogleClientSecret, w.Field == 1, true, th))
	b.WriteString("\n")
	b.WriteString(muted.Render("Tab to switch fields. Enter to continue."))
	return b.String()
}

func (w ImportSetup) viewSpotifyRedirect(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	good := lipgloss.NewStyle().Foreground(th.Success)
	var b strings.Builder
	b.WriteString(accent.Render("Spotify — Verify Your Existing App") + "\n")
	b.WriteString(body.Render("You already created a Spotify Developer app during the first-run playback setup. Import reuses that same app — no need to create a second one."))
	b.WriteString("\n\n")
	b.WriteString(muted.Render("Press O to open:"))
	b.WriteString("\n  " + accent.Render(w.URLForStep()))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Click your existing musicTUI app → Settings → Edit. Two things to confirm:"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("  1. Redirect URIs includes:"))
	b.WriteString("\n     " + good.Render("✓ http://127.0.0.1:8888/callback"))
	b.WriteString("\n     " + muted.Render("(This is the same port playback uses, so it"))
	b.WriteString("\n     " + muted.Render(" should already be there.)"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("  2. Under User Management, your Spotify account is"))
	b.WriteString("\n     " + body.Render("listed as a user — required for playlist creation."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("If anything's missing, add it and click Save."))
	return b.String()
}

func (w ImportSetup) viewSpotifyPasteSecret(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	var b strings.Builder
	b.WriteString(accent.Render("Paste Spotify Client Secret") + "\n")
	b.WriteString(body.Render("Your Spotify Client ID is already configured (from playback setup). The import flow needs the Client Secret too."))
	b.WriteString("\n\n")
	b.WriteString(body.Render("In your Spotify app's dashboard, click \"View Client Secret\" (or generate one if it's not visible) and paste below."))
	b.WriteString("\n\n")
	b.WriteString(w.renderField("Client Secret", w.SpotifyClientSecret, true, true, th))
	b.WriteString("\n")
	b.WriteString(muted.Render("Press Enter to save and finish."))
	return b.String()
}

func (w ImportSetup) viewDone(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(64)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(64)
	good := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	warn := lipgloss.NewStyle().Foreground(th.Warning).Bold(true)
	var b strings.Builder
	b.WriteString(good.Render("✓ Setup complete"))
	b.WriteString("\n\n")
	b.WriteString(body.Render("Credentials saved to ~/.config/musicTUI/config.toml."))
	b.WriteString("\n" + body.Render("OAuth tokens will be encrypted and stored at"))
	b.WriteString("\n" + muted.Render("~/.config/musicTUI/import/") + ".")
	b.WriteString("\n\n")
	b.WriteString(accent.Render("Heads-up on the Google sign-in screen:") + "\n")
	b.WriteString(body.Render("When you start the import, your browser will open a Google sign-in page that may say:"))
	b.WriteString("\n\n   " + warn.Render("\"Google hasn't verified this app\""))
	b.WriteString("\n\n")
	b.WriteString(body.Render("That's expected — you are the developer of the app you just created. Click the small"))
	b.WriteString(" " + accent.Render("Continue") + " " + body.Render("link (not \"Back to safety\"), then approve the YouTube permissions. Google shows this to anyone running an unverified app; it's only a warning, not a block."))
	b.WriteString("\n\n")
	b.WriteString(accent.Render("What happens next:") + "\n")
	b.WriteString(body.Render("Press Enter to return to the Import view. Then Enter again to start the import — browser opens for YouTube first, then Spotify."))
	return b.String()
}

// renderField draws a single labeled text-input box. Active=true
// shows the cursor; secret=true masks the typed characters.
//
// All string indexing is rune-aware so the masked secret display (one
// `•` per input rune) aligns with CursorPos (which is also in rune
// units). A byte-based slice here was a bug — it cut multi-byte UTF-8
// characters mid-sequence and produced garbled output in the box.
func (w ImportSetup) renderField(label, value string, active, secret bool, th theme.Theme) string {
	labelStyle := lipgloss.NewStyle().Foreground(th.FgMuted)

	var displayRunes []rune
	if secret {
		displayRunes = make([]rune, utf8.RuneCountInString(value))
		for i := range displayRunes {
			displayRunes[i] = '*'
		}
	} else {
		displayRunes = []rune(value)
	}

	var display string
	if active {
		pos := w.CursorPos
		if pos > len(displayRunes) {
			pos = len(displayRunes)
		}
		cursor := lipgloss.NewStyle().Background(th.Accent).Render(" ")
		display = string(displayRunes[:pos]) + cursor + string(displayRunes[pos:])
		if len(displayRunes) == 0 {
			display = cursor
		}
	} else {
		display = string(displayRunes)
	}

	border := th.Border
	if active {
		border = th.Accent
	}
	field := lipgloss.NewStyle().
		Foreground(th.Fg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(64).
		Padding(0, 1).
		Render(display)
	return labelStyle.Render("  "+label) + "\n" + field
}
