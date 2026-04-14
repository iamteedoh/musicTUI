package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
	"github.com/iamteedoh/musicTUI/internal/ytmusic"
)

// ImportSource identifies which upstream the user picked to import
// from. Set in ImportStageIdle when the user chooses and read by the
// app's key handler to route into the right command.
type ImportSource int

const (
	SourceNone   ImportSource = iota
	SourceYT     // YouTube Music
	SourceApple  // Apple Music
)

// ImportStage is the high-level state of the import view. The app
// transitions between stages as async commands (device-code request,
// auth polling, library reads, the import itself) complete.
type ImportStage int

const (
	// ImportStageIdle: user hasn't started anything yet. Shows a
	// source picker (YouTube Music / Apple Music).
	ImportStageIdle ImportStage = iota
	// ImportStageDeviceCode: device-flow in progress. We have a user
	// code to show; we're polling Google every N seconds for approval.
	ImportStageDeviceCode
	// ImportStageAuthed: have a valid token but haven't loaded the
	// library yet.
	ImportStageAuthed
	// ImportStageLoadingLibrary: actively fetching library from YT.
	ImportStageLoadingLibrary
	// ImportStageLibraryLoaded: shows how many playlists / liked songs
	// / albums were found, with a prompt to start the import.
	ImportStageLibraryLoaded
	// ImportStageImporting: the import is running. Progress text shows
	// which playlist is being matched and how far along we are.
	ImportStageImporting
	// ImportStageDone: finished. Shows per-playlist match counts and
	// unmatched-track tally so the user can spot drops.
	ImportStageDone
	// ImportStageError: an unrecoverable error during any of the
	// stages above. User can retry by pressing Enter.
	ImportStageError
)

// Import is the state + renderer for the Import screen. Most fields
// are populated by the app's message handlers as async commands land.
type Import struct {
	Stage  ImportStage
	Source ImportSource // chosen during Idle; drives which command runs

	// Source-selector state — which option is highlighted in Idle
	Selected int // 0 = YouTube Music, 1 = Apple Music

	// Apple Music specific: true iff AppleMusic config is populated
	AppleConfigured bool

	// Device-flow display
	UserCode        string
	VerificationURL string

	// Library summary (populated on ImportStageLibraryLoaded)
	Playlists    []ytmusic.Playlist
	LikedCount   int
	Albums       []ytmusic.Album
	Artists      []ytmusic.Artist

	// Progress (populated during ImportStageImporting)
	ProgressCurrentPlaylist string
	ProgressDone            int
	ProgressTotal           int
	ProgressOverall         int
	ProgressOverallTotal    int

	// Terminal states
	Summaries []ytmusic.ImportSummary
	Err       error
}

func NewImport() Import {
	return Import{Stage: ImportStageIdle}
}

// Reset clears everything back to the idle state. Called on close-and-
// reopen so re-running an import starts clean. AppleConfigured stays
// since it's a derived config flag, not session state.
func (i *Import) Reset() {
	apple := i.AppleConfigured
	*i = Import{Stage: ImportStageIdle, AppleConfigured: apple}
}

// SelectUp / SelectDown move the highlight in the source picker.
func (i *Import) SelectUp() {
	if i.Stage == ImportStageIdle && i.Selected > 0 {
		i.Selected--
	}
}
func (i *Import) SelectDown() {
	if i.Stage == ImportStageIdle && i.Selected < 1 {
		i.Selected++
	}
}

func (i Import) View(th theme.Theme, width, height int) string {
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
		Render("Import from YouTube Music")
	sub := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
		Render("One-time import of your YT Music library into Spotify.")

	var body string
	switch i.Stage {
	case ImportStageIdle:
		body = i.viewIdle(th)
	case ImportStageDeviceCode:
		body = i.viewDeviceCode(th)
	case ImportStageAuthed:
		body = i.viewAuthed(th)
	case ImportStageLoadingLibrary:
		body = i.viewLoadingLibrary(th)
	case ImportStageLibraryLoaded:
		body = i.viewLibraryLoaded(th)
	case ImportStageImporting:
		body = i.viewImporting(th, width)
	case ImportStageDone:
		body = i.viewDone(th, width)
	case ImportStageError:
		body = i.viewError(th)
	}

	return " " + title + "\n " + sub + "\n\n" + body
}

// ─────────────────── per-stage views ───────────────────

func (i Import) viewIdle(th theme.Theme) string {
	body := lipgloss.NewStyle().Foreground(th.Fg).Width(66)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted).Width(66)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	selected := lipgloss.NewStyle().
		Foreground(th.Surface).
		Background(th.Accent).
		Bold(true).
		Padding(0, 1)
	unselected := lipgloss.NewStyle().Foreground(th.Fg).Padding(0, 1)

	var b strings.Builder
	b.WriteString(" " + body.Render("Import your playlists and library from another streaming service into Spotify. This runs on demand — nothing happens automatically."))
	b.WriteString("\n\n")
	b.WriteString(" " + accent.Render("Choose a source:"))
	b.WriteString("\n\n")

	// Source picker
	opts := []struct {
		label string
		hint  string
		ready bool
	}{
		{"YouTube Music", "one-time sign-in with a short code", true},
		{"Apple Music", "", i.AppleConfigured},
	}
	if opts[1].ready {
		opts[1].hint = "one-time sign-in via your Apple Developer page"
	} else {
		opts[1].hint = "⚠ needs setup — see README (Apple Music Setup)"
	}

	for idx, opt := range opts {
		marker := "  "
		labelStyle := unselected
		if idx == i.Selected {
			marker = "▸ "
			labelStyle = selected
		}
		line := " " + marker + labelStyle.Render(opt.label)
		if opt.hint != "" {
			line += "  " + muted.Render(opt.hint)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n " + muted.Render("Imported playlists are created with a prefix ([YT] or [AM]) so you can spot them in your Spotify sidebar and clean up later if anything goes wrong."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"j/k · ↑↓", "pick"},
		{"Enter", "start"},
		{"Esc", "back"},
	}))
	return b.String()
}

func (i Import) viewDeviceCode(th theme.Theme) string {
	label := lipgloss.NewStyle().Foreground(th.FgMuted)
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	code := lipgloss.NewStyle().
		Foreground(th.Fg).
		Background(th.Accent).
		Bold(true).
		Padding(0, 2).
		Render(" " + i.UserCode + " ")

	var b strings.Builder
	b.WriteString(" " + label.Render("On any device, open:") + "\n")
	b.WriteString(" " + accent.Render(i.VerificationURL) + "\n\n")
	b.WriteString(" " + label.Render("Enter this code:") + "\n")
	b.WriteString(" " + code + "\n\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
		Render("◌ Waiting for approval... (this view will update automatically)"))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Esc", "cancel"},
	}))
	return b.String()
}

func (i Import) viewAuthed(th theme.Theme) string {
	return " " + lipgloss.NewStyle().Foreground(th.Success).Render("✓ Connected to YouTube Music") +
		"\n\n " + lipgloss.NewStyle().Foreground(th.FgDim).Render("Press Enter to fetch your library summary.") +
		"\n\n " + RenderHints(th, []Hint{
			{"Enter", "load library"},
			{"Esc", "back"},
		})
}

func (i Import) viewLoadingLibrary(th theme.Theme) string {
	return " " + lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ") +
		lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).
			Render("Fetching your YouTube Music library...")
}

func (i Import) viewLibraryLoaded(th theme.Theme) string {
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Library found:") + "\n\n")
	b.WriteString(fmt.Sprintf("   %s  %d playlists\n",
		accent.Render("♪"), len(i.Playlists)))
	b.WriteString(fmt.Sprintf("   %s  %d liked songs\n",
		accent.Render("♥"), i.LikedCount))
	b.WriteString(fmt.Sprintf("   %s  %d saved albums\n",
		accent.Render("◎"), len(i.Albums)))
	b.WriteString(fmt.Sprintf("   %s  %d followed artists\n",
		accent.Render("♧"), len(i.Artists)))
	b.WriteString("\n " + muted.Render("Imported playlists will be prefixed with [YT] so you can spot them later. Nothing on Spotify gets deleted or modified except new playlists being created."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "start import"},
		{"Esc", "cancel"},
	}))
	return b.String()
}

func (i Import) viewImporting(th theme.Theme, width int) string {
	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)

	var b strings.Builder
	b.WriteString(" " + accent.Render("Importing... ") +
		dim.Render("(please don't close the app)") + "\n\n")

	if i.ProgressOverallTotal > 0 {
		overall := fmt.Sprintf("Playlist %d of %d", i.ProgressOverall, i.ProgressOverallTotal)
		b.WriteString(" " + accent.Render(overall) + "\n")
	}
	if i.ProgressCurrentPlaylist != "" {
		b.WriteString(" " + dim.Render("▸ "+truncate(i.ProgressCurrentPlaylist, 40)) + "\n")
	}
	if i.ProgressTotal > 0 {
		bar := renderProgressBar(th, i.ProgressDone, i.ProgressTotal, width-10)
		b.WriteString(" " + bar + " " +
			dim.Render(fmt.Sprintf("%d / %d", i.ProgressDone, i.ProgressTotal)))
	}
	return b.String()
}

func (i Import) viewDone(th theme.Theme, width int) string {
	success := lipgloss.NewStyle().Foreground(th.Success).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)

	var totalMatched, totalUnmatched, totalErrors int
	for _, s := range i.Summaries {
		totalMatched += s.MatchedCount
		totalUnmatched += s.UnmatchedCount
		totalErrors += s.ErrorCount
	}

	var b strings.Builder
	b.WriteString(" " + success.Render("✓ Import complete") + "\n\n")
	b.WriteString(fmt.Sprintf("   %d playlists imported\n", len(i.Summaries)))
	b.WriteString(fmt.Sprintf("   %d tracks matched\n", totalMatched))
	if totalUnmatched > 0 {
		warn := lipgloss.NewStyle().Foreground(th.Warning)
		b.WriteString("   " + warn.Render(fmt.Sprintf("%d tracks not found on Spotify", totalUnmatched)) + "\n")
	}
	if totalErrors > 0 {
		errStyle := lipgloss.NewStyle().Foreground(th.Error)
		b.WriteString("   " + errStyle.Render(fmt.Sprintf("%d search errors", totalErrors)) + "\n")
	}
	b.WriteString("\n " + muted.Render("Your imported playlists are in the sidebar now (prefixed with [YT])."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "run another import"},
		{"Esc", "back"},
	}))
	return b.String()
}

func (i Import) viewError(th theme.Theme) string {
	errStyle := lipgloss.NewStyle().Foreground(th.Error).Bold(true)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)
	msg := "Unknown error"
	if i.Err != nil {
		msg = i.Err.Error()
	}
	var b strings.Builder
	b.WriteString(" " + errStyle.Render("✗ Import failed") + "\n\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Fg).Width(66).Render(msg) + "\n\n")
	b.WriteString(" " + muted.Italic(true).Render("If this keeps happening, check your network and that the device-code approval actually went through."))
	b.WriteString("\n\n " + RenderHints(th, []Hint{
		{"Enter", "retry"},
		{"Esc", "back"},
	}))
	return b.String()
}

// ─────────────────── helpers ───────────────────

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
