package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/importbackend"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// ImportStage is the high-level state of the Import screen. The app
// transitions stages as RPCs land and the local importer emits events.
type ImportStage int

const (
	// ImportStageNotConfigured: user hasn't set up OAuth client creds
	// yet (Import.GoogleClientID is empty). Show config instructions
	// instead of the normal flow.
	ImportStageNotConfigured ImportStage = iota
	// ImportStageIdle: ready to start — show the intro + Enter prompt.
	ImportStageIdle
	// ImportStageEnsuringSession: hitting POST /api/session on first
	// run, or validating a cached session_id on relaunch.
	ImportStageEnsuringSession
	// ImportStageAwaitingAuth: backend session exists but at least one
	// of {youtube, spotify} isn't connected yet. We've opened the
	// browser at /auth/<service>/start and we're polling the session
	// state until both flip to true.
	ImportStageAwaitingAuth
	// ImportStageLoadingLibrary: GET /library/youtube in flight.
	ImportStageLoadingLibrary
	// ImportStageLibraryLoaded: shows the playlist + liked counts and
	// asks the user to confirm before kicking the import.
	ImportStageLibraryLoaded
	// ImportStageImporting: SSE stream is active. Per-track / per-
	// playlist progress shows in real time.
	ImportStageImporting
	// ImportStageDone: terminal success.
	ImportStageDone
	// ImportStageError: terminal failure.
	ImportStageError
)

// Import is the state + renderer for the Import screen.
type Import struct {
	Stage ImportStage

	// Auth phase
	YouTubeConnected     bool
	SpotifyConnected     bool
	AuthBrowserOpenedFor string // "youtube" | "spotify" | "" — last service we triggered for

	// Library phase
	Playlists  []importbackend.PlaylistSummary
	LikedCount int

	// Import phase (streamed via the importer event channel)
	ProgressCurrentPlaylist string
	ProgressDone            int
	ProgressTotal           int
	ProgressOverall         int // playlists finished
	ProgressOverallTotal    int // total playlists in this run (incl. Liked)

	// Terminal results
	Matched   int
	Unmatched int
	Errors    int
	JobURL    string // open.spotify.com URL of the most-recent created playlist (last seen)

	// ErrorReasons counts each distinct error message seen during the
	// import. Displayed on the Done screen so users can see what
	// actually went wrong (e.g. "spotify: ... 429: API rate limit
	// exceeded") rather than just a blind number.
	ErrorReasons map[string]int

	Err error
}

type ImportErrorAdvice struct {
	Service string
	Lines   []string
}

func NewImport() Import {
	return Import{Stage: ImportStageIdle}
}

// MarkNotConfigured is called by the app at startup if the user
// hasn't supplied OAuth credentials yet. Switches the view to the
// "setup required" stage instead of the normal Idle.
func (i *Import) MarkNotConfigured() {
	i.Stage = ImportStageNotConfigured
}

// Reset clears everything back to the idle state — called when the
// user presses Enter from a terminal state to run another import, and
// when they navigate away.
func (i *Import) Reset() { *i = Import{Stage: ImportStageIdle} }

// BothConnected reports whether the user has finished both OAuth
// flows for the current import. The view advances to the
// "library" stage automatically once this is true.
func (i Import) BothConnected() bool { return i.YouTubeConnected && i.SpotifyConnected }

// NextServiceToConnect names the service the user still needs to
// auth — drives which URL we open in their browser. Returns "" if
// both are connected.
func (i Import) NextServiceToConnect() string {
	if !i.YouTubeConnected {
		return "youtube"
	}
	if !i.SpotifyConnected {
		return "spotify"
	}
	return ""
}

func (i Import) View(th theme.Theme, width, height int) string {
	// Reserve one char of inset on either side so long lines don't
	// collide with the surrounding panel border.
	inner := width - 2
	if inner < 20 {
		inner = 20
	}

	wrap := lipgloss.NewStyle().Width(inner)

	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
		Render("Import from YouTube Music")
	sub := wrap.Foreground(th.FgMuted).Italic(true).
		Render("Local transfer into Spotify.")

	var body string
	switch i.Stage {
	case ImportStageNotConfigured:
		body = i.viewNotConfigured(th, inner)
	case ImportStageIdle:
		body = i.viewIdle(th, inner)
	case ImportStageEnsuringSession:
		body = i.viewEnsuringSession(th)
	case ImportStageAwaitingAuth:
		body = i.viewAwaitingAuth(th, inner)
	case ImportStageLoadingLibrary:
		body = i.viewLoadingLibrary(th)
	case ImportStageLibraryLoaded:
		body = i.viewLibraryLoaded(th, inner)
	case ImportStageImporting:
		body = i.viewImporting(th, width)
	case ImportStageDone:
		body = i.viewDone(th, width)
	case ImportStageError:
		body = i.viewError(th, inner)
	}

	return " " + title + "\n " + sub + "\n\n" + body
}

// ─────────────────── per-stage views ───────────────────

func (i Import) viewNotConfigured(th theme.Theme, w int) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(w)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Import — first-time setup") + "\n\n")
	b.WriteString(" " + body.Render("The library import feature needs OAuth credentials for Google Cloud (YouTube) and Spotify. Each user creates their own free OAuth apps — no tokens ever leave your machine, no shared quota, no signup."))
	b.WriteString("\n\n")
	b.WriteString(" " + body.Render("Press Enter to launch a step-by-step setup wizard. It takes about 10 minutes and walks you through every click."))
	b.WriteString("\n\n")
	b.WriteString(" " + muted.Render("You can press Esc to cancel the wizard at any point — partial progress is preserved."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "start setup wizard"},
		{"Esc", "back"},
	}))
	return b.String()
}

func (i Import) viewIdle(th theme.Theme, w int) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(w)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)

	var b strings.Builder
	b.WriteString(" " + body.Render("Import your YouTube Music playlists into Spotify."))
	b.WriteString("\n\n")
	b.WriteString(" " + accent.Render("How it works:") + "\n")
	for _, line := range []string{
		"1. Sign in to YouTube and Spotify in your browser (one-time).",
		"2. We read your YT playlists and create matching Spotify playlists.",
		"3. Watch real-time progress; nothing existing is touched.",
	} {
		b.WriteString(" " + muted.Render(line) + "\n")
	}
	b.WriteString("\n " + muted.Render("Runs entirely on your machine — no hosted backend. Tokens are stored locally under the musicTUI config dir."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "begin"},
		{"c", "reconfigure"},
		{"Esc", "back"},
	}))
	return b.String()
}

func (i Import) viewEnsuringSession(th theme.Theme) string {
	return " " + lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ") +
		lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).
			Render("Connecting to import backend...")
}

func (i Import) viewAwaitingAuth(th theme.Theme, w int) string {
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	ok := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	wait := lipgloss.NewStyle().Foreground(th.FgDim).Italic(true)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Connect your accounts:") + "\n\n")

	for _, svc := range []struct {
		key, label string
		done       bool
	}{
		{"youtube", "YouTube Music", i.YouTubeConnected},
		{"spotify", "Spotify", i.SpotifyConnected},
	} {
		marker := "○"
		state := wait.Render("waiting...")
		if svc.done {
			marker = "✓"
			state = ok.Render("connected")
		} else if i.AuthBrowserOpenedFor == svc.key {
			marker = "◌"
			state = wait.Render("waiting for browser sign-in...")
		}
		b.WriteString(fmt.Sprintf("   %s  %s   %s\n",
			lipgloss.NewStyle().Foreground(th.Accent).Render(marker),
			svc.label, state))
	}

	b.WriteString("\n " + muted.Render("Your default browser should have opened. If not, copy the URL printed in the status bar."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"r", "re-open browser"},
		{"Esc", "cancel"},
	}))
	return b.String()
}

func (i Import) viewLoadingLibrary(th theme.Theme) string {
	return " " + lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ") +
		lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).
			Render("Reading your YouTube Music library...")
}

func (i Import) viewLibraryLoaded(th theme.Theme, w int) string {
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Library found:") + "\n\n")
	b.WriteString(fmt.Sprintf("   %s  %d YouTube playlists\n",
		accent.Render("♪"), len(i.Playlists)))
	b.WriteString("\n " + muted.Render("This counts EVERY playlist on your YouTube account — including regular video playlists — so it may be far more than your music playlists. Don't worry: each playlist is checked track by track and only actual music (YouTube's Music category) is imported. Playlists with no music are skipped entirely and never reach Spotify."))
	b.WriteString("\n\n " + muted.Render("Each imported playlist becomes a new Spotify playlist with the same name. Existing Spotify playlists are not touched. Tracks that can't be confidently matched are reported as unmatched rather than replaced with a wrong version."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "start import"},
		{"Esc", "cancel"},
	}))
	return b.String()
}

func (i Import) viewImporting(th theme.Theme, width int) string {
	w := width - 2
	if w < 20 {
		w = 20
	}
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgDim).Width(w)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Importing... ") +
		lipgloss.NewStyle().Foreground(th.FgDim).Render("(safe to leave open)") + "\n\n")

	if i.ProgressOverallTotal > 0 {
		b.WriteString(" " + accent.Render(
			fmt.Sprintf("Playlist %d of %d", i.ProgressOverall, i.ProgressOverallTotal),
		) + "\n")
	}
	if i.ProgressCurrentPlaylist != "" {
		b.WriteString(" " + lipgloss.NewStyle().Foreground(th.FgDim).
			Render("▸ "+truncate(i.ProgressCurrentPlaylist, w-4)) + "\n")
	}
	if i.ProgressTotal > 0 {
		barWidth := w - 14
		if barWidth < 10 {
			barWidth = 10
		}
		bar := renderProgressBar(th, i.ProgressDone, i.ProgressTotal, barWidth)
		b.WriteString(" " + bar + " " +
			lipgloss.NewStyle().Foreground(th.FgDim).
				Render(fmt.Sprintf("%d / %d", i.ProgressDone, i.ProgressTotal)))
	}
	b.WriteString("\n\n " + dim.Render(
		fmt.Sprintf("running tally — matched: %d  unmatched: %d  errors: %d",
			i.Matched, i.Unmatched, i.Errors)))
	// Show the most recent / most common error mid-import so the user
	// can tell whether to let it run or Esc out and investigate. 100%
	// error rate in the first dozen tracks almost always means
	// something structural (auth, scopes, app config), not transient.
	if i.Errors > 0 {
		errStyle := lipgloss.NewStyle().Foreground(th.Error).Width(w)
		if top := i.topErrorReasons(1); len(top) > 0 {
			b.WriteString("\n\n " + errStyle.Render("⚠ "+top[0]))
		}
	}
	return b.String()
}

func (i Import) viewDone(th theme.Theme, width int) string {
	w := width - 2
	if w < 20 {
		w = 20
	}
	// Indented rows get 3 chars of leading space ("   "), so wrap at w-3.
	indented := w - 3
	if indented < 15 {
		indented = 15
	}
	success := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)

	var b strings.Builder
	b.WriteString(" " + success.Render("✓ Import complete") + "\n\n")
	b.WriteString(fmt.Sprintf("   %d playlists imported\n", i.ProgressOverallTotal))
	b.WriteString(fmt.Sprintf("   %d tracks matched\n", i.Matched))
	if i.Unmatched > 0 {
		warn := lipgloss.NewStyle().Foreground(th.Warning).Width(indented)
		b.WriteString("   " + warn.Render(
			fmt.Sprintf("%d tracks not found on Spotify (likely non-music videos or rare tracks)", i.Unmatched),
		) + "\n")
	}
	if i.Errors > 0 {
		errStyle := lipgloss.NewStyle().Foreground(th.Error).Width(indented)
		b.WriteString("   " + errStyle.Render(
			fmt.Sprintf("%d search errors", i.Errors),
		) + "\n")
		// Show the top error reasons so users can tell whether it's
		// something they should act on (rate limiting, token expiry,
		// actually bad data) rather than a mystery number.
		if top := i.topErrorReasons(3); len(top) > 0 {
			b.WriteString("\n " + muted.Render("Most common causes:") + "\n")
			for _, line := range top {
				b.WriteString("   " + muted.Render("• "+line) + "\n")
			}
		}
	}
	b.WriteString("\n " + muted.Render("Your imported playlists are in the sidebar. Go to the Playlists view to refresh the list."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "run another import"},
		{"Esc", "back"},
	}))
	return b.String()
}

func (i Import) viewError(th theme.Theme, w int) string {
	errStyle := lipgloss.NewStyle().Foreground(th.Error).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(w)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	msg := "Unknown error"
	if i.Err != nil && i.Err.Error() != "" {
		msg = i.Err.Error()
	}
	advice := ImportErrorAdviceFor(i.Err)
	var b strings.Builder
	b.WriteString(" " + errStyle.Render("✗ Import failed") + "\n\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Fg).Width(w).Render(msg) + "\n\n")
	if len(advice.Lines) > 0 {
		b.WriteString(" " + accent.Render("How to fix:") + "\n")
		for _, line := range advice.Lines {
			b.WriteString(" " + muted.Render("• "+line) + "\n")
		}
	} else {
		b.WriteString(" " + muted.Italic(true).Render("Check your network, then press Enter to retry. If it repeats, press c to reconfigure import credentials."))
	}
	hints := []Hint{
		{"Enter", "retry"},
	}
	if advice.Service != "" {
		hints = append(hints, Hint{"r", "reconnect " + serviceDisplayName(advice.Service)})
	}
	hints = append(hints,
		Hint{"c", "reconfigure"},
		Hint{"Esc", "back"},
	)
	b.WriteString("\n\n " + RenderHints(th, hints))
	return b.String()
}

// ─────────────────── helpers ───────────────────

func ImportErrorAdviceFor(err error) ImportErrorAdvice {
	if err == nil {
		return ImportErrorAdvice{}
	}
	raw := err.Error()
	msg := strings.ToLower(raw)
	service := inferImportErrorService(msg)

	switch {
	case strings.Contains(msg, "invalid_grant"):
		if service == "" {
			service = "youtube"
		}
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				fmt.Sprintf("Your saved %s login can no longer be refreshed.", serviceDisplayName(service)),
				"Press r to reconnect in your browser.",
				"If reconnecting fails, press c and verify the OAuth client settings.",
			},
		}
	case strings.Contains(msg, "invalid_client") || strings.Contains(msg, "unauthorized_client"):
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				"The OAuth client ID or client secret is being rejected.",
				"Press c to re-run setup and paste the current OAuth app values.",
				"Make sure the redirect URI in the provider dashboard matches the wizard.",
			},
		}
	case strings.Contains(msg, "access_denied"):
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				"Browser authorization was denied or canceled.",
				"Press r to open the sign-in flow again and approve the requested access.",
			},
		}
	case strings.Contains(msg, "403") || strings.Contains(msg, "forbidden"):
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				"The provider refused access for the current app/account.",
				"For YouTube, enable YouTube Data API v3 and add your account as a test user.",
				"Then press r to reconnect, or c to reconfigure.",
			},
		}
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				"The provider is rate limiting requests.",
				"Wait before retrying, or use a dedicated Spotify import app if Spotify is the limiter.",
			},
		}
	case strings.Contains(msg, "network") || strings.Contains(msg, "timeout") || strings.Contains(msg, "connection refused"):
		return ImportErrorAdvice{
			Service: service,
			Lines: []string{
				"Check your network connection and provider availability.",
				"Press Enter to retry when the connection is stable.",
			},
		}
	}
	return ImportErrorAdvice{}
}

func inferImportErrorService(msg string) string {
	switch {
	case strings.Contains(msg, "youtube") || strings.Contains(msg, "google"):
		return "youtube"
	case strings.Contains(msg, "spotify"):
		return "spotify"
	default:
		return ""
	}
}

func serviceDisplayName(service string) string {
	switch service {
	case "youtube":
		return "YouTube"
	case "spotify":
		return "Spotify"
	default:
		return "service"
	}
}

// topErrorReasons returns up to `limit` error strings ordered by
// frequency (most common first). Each string is truncated and has
// its count suffixed so the Done screen shows, e.g.:
//
//   - 429: API rate limit exceeded  (174x)
func (i Import) topErrorReasons(limit int) []string {
	if len(i.ErrorReasons) == 0 {
		return nil
	}
	type pair struct {
		reason string
		count  int
	}
	all := make([]pair, 0, len(i.ErrorReasons))
	for r, c := range i.ErrorReasons {
		all = append(all, pair{r, c})
	}
	// Simple insertion-sort by count desc; set never gets large.
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].count > all[j-1].count; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	out := make([]string, 0, limit)
	for k := 0; k < len(all) && k < limit; k++ {
		r := all[k].reason
		if len(r) > 70 {
			r = r[:69] + "…"
		}
		out = append(out, fmt.Sprintf("%s  (%d×)", r, all[k].count))
	}
	return out
}

func renderProgressBar(th theme.Theme, done, total, width int) string {
	if width < 10 {
		width = 10
	}
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(th.FgMuted).Render(strings.Repeat("░", width))
	}
	filled := width * done / total
	if filled > width {
		filled = width
	}
	return th.GradientProgress(filled, width)
}
