package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type NowPlaying struct{}

func NewNowPlaying() NowPlaying {
	return NowPlaying{}
}

func fmtDuration(ms int) string {
	s := ms / 1000
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// StatusBarView renders a single-line status bar at the bottom of the screen.
// Left side: static keybinding legend covering all core actions. Right
// side: playback info + progress + volume. Left side gracefully shortens
// to a compact variant on narrow terminals so it never pushes the
// right side off-screen.
func (n NowPlaying) StatusBarView(th theme.Theme, pb model.PlaybackState, width int) string {
	dim := lipgloss.NewStyle().Foreground(th.FgMuted)
	bright := lipgloss.NewStyle().Foreground(th.Fg)

	// ── Right: now-playing info ── (compute first so we know how much
	// space is left for the legend on the left)
	var right string
	if pb.Track != nil {
		trackInfo := bright.Bold(true).Render("♫ " + truncate(pb.Track.Name, 25))
		artistInfo := dim.Render(" — " + truncate(pb.Track.ArtistNames(), 20))

		// Mini progress bar
		posMs := int(pb.Position.Milliseconds())
		durMs := int(pb.Track.Duration.Milliseconds())
		timeStr := fmtDuration(posMs) + "/" + fmtDuration(durMs)

		barW := 15
		filled := 0
		if durMs > 0 {
			filled = barW * posMs / durMs
		}
		if filled > barW {
			filled = barW
		}
		progressBar := th.GradientProgress(filled, barW)

		// Volume
		volIcon := "󰕾"
		if pb.Volume == 0 {
			volIcon = "󰖁"
		} else if pb.Volume < 30 {
			volIcon = "󰕿"
		}
		vol := dim.Render(fmt.Sprintf(" %s%d%%", volIcon, pb.Volume))

		right = trackInfo + artistInfo + "  " + progressBar + " " +
			dim.Render(timeStr) + vol
	} else {
		right = dim.Italic(true).Render("No track playing")
	}

	// ── Left: comprehensive static legend ──
	// Tiers of progressively-shorter hint sets so we degrade gracefully
	// rather than truncating mid-word when the terminal is narrow.
	full := []Hint{
		{"Space", "⏯"},
		{"n/p", "⏭/⏮"},
		{"+/-", "vol"},
		{"s", "shuffle"},
		{"r", "repeat"},
		{"l", "lyrics"},
		{"/", "search"},
		{"?", "help"},
		{"q", "quit"},
	}
	medium := []Hint{
		{"Space", "⏯"},
		{"n/p", "⏭/⏮"},
		{"+/-", "vol"},
		{"/", "search"},
		{"?", "help"},
		{"q", "quit"},
	}
	short := []Hint{
		{"Space", "⏯"},
		{"n/p", "⏭/⏮"},
		{"?", "help"},
	}
	rightW := lipgloss.Width(right)
	available := width - rightW - 4 // padding + inter-section gap

	var hints string
	for _, candidate := range [][]Hint{full, medium, short} {
		h := RenderHints(th, candidate)
		if lipgloss.Width(h) <= available {
			hints = h
			break
		}
	}
	if hints == "" {
		// Fallback — terminal is very narrow; show just the marker
		hints = dim.Render("?: help")
	}

	// ── Compose status bar ──
	leftW := lipgloss.Width(hints)
	gap := width - leftW - rightW - 2
	if gap < 1 {
		gap = 1
	}

	bar := " " + hints + strings.Repeat(" ", gap) + right + " "

	return lipgloss.NewStyle().
		Width(width).
		Background(th.SurfaceBright).
		Foreground(th.FgDim).
		Render(bar)
}

// PanelContent renders now-playing info for the center panel.
// Shows track name, artist, album, progress bar, and controls.
func (n NowPlaying) PanelContent(th theme.Theme, pb model.PlaybackState, width int) string {
	if pb.Track == nil {
		return lipgloss.NewStyle().
			Foreground(th.FgMuted).Italic(true).
			Render("  No track playing — press / to search")
	}

	accent := lipgloss.NewStyle().Foreground(th.Accent)
	bright := lipgloss.NewStyle().Foreground(th.Fg).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)

	// Track name (bold, bright)
	trackLine := " " + bright.Render("**"+pb.Track.Name+"**")

	// Artist — Album
	artistAlbum := " " + dim.Render(pb.Track.ArtistNames())
	if pb.Track.Album != nil {
		artistAlbum += muted.Render(" — ") + dim.Render(pb.Track.Album.Name)
	}

	// Progress bar
	posMs := int(pb.Position.Milliseconds())
	durMs := int(pb.Track.Duration.Milliseconds())
	timeStr := fmtDuration(posMs) + " / " + fmtDuration(durMs)

	playIcon := accent.Render("▶")
	if !pb.IsPlaying {
		playIcon = muted.Render("⏸")
	}

	barW := width - 20
	if barW < 10 {
		barW = 10
	}
	filled := 0
	if durMs > 0 {
		filled = barW * posMs / durMs
	}
	if filled > barW {
		filled = barW
	}

	progressLine := " [" + playIcon + " " + muted.Render(timeStr) + "] " +
		th.GradientProgress(filled, barW)

	// Controls
	shuffleIcon := muted.Render("󰒟")
	if pb.Shuffle {
		shuffleIcon = accent.Render("󰒟")
	}
	repeatIcon := muted.Render("󰑖")
	switch pb.Repeat {
	case model.RepeatContext:
		repeatIcon = accent.Render("󰑖")
	case model.RepeatTrack:
		repeatIcon = accent.Bold(true).Render("󰑘")
	}
	controlLine := strings.Repeat(" ", width-10) + shuffleIcon + "  " + repeatIcon

	return trackLine + "\n" + artistAlbum + "\n" + progressLine + "\n" + controlLine
}

// View is the old multi-line now-playing panel (kept for compatibility but unused).
func (n NowPlaying) View(th theme.Theme, pb model.PlaybackState, width int) string {
	return n.StatusBarView(th, pb, width)
}
