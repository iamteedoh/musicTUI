package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/lyrics"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type Lyrics struct {
	Lines     []lyrics.LyricLine
	PlainText string
	TrackID   string // which track these lyrics are for
	Synced    bool
	Loading   bool
	Error     string
	ScrollPos int // for manual scrolling of plain lyrics
}

func NewLyrics() Lyrics {
	return Lyrics{}
}

func (l *Lyrics) SetLyrics(result *lyrics.Result, trackID string) {
	l.TrackID = trackID
	l.Loading = false
	l.Error = ""
	l.ScrollPos = 0
	if result == nil || (!result.Synced && result.Plain == "") {
		l.Lines = nil
		l.PlainText = ""
		l.Synced = false
		return
	}
	l.Lines = result.Lines
	l.PlainText = result.Plain
	l.Synced = result.Synced
}

func (l *Lyrics) SetError(err string) {
	l.Loading = false
	l.Error = err
}

func (l *Lyrics) ScrollUp() {
	if l.ScrollPos > 0 {
		l.ScrollPos--
	}
}

func (l *Lyrics) ScrollDown() {
	l.ScrollPos++
}

// View renders lyrics. For synced lyrics, highlights the current line
// based on playback position and auto-scrolls.
func (l Lyrics) View(th theme.Theme, width, height int, positionMs int64) string {
	if l.Loading {
		return lipgloss.NewStyle().Foreground(th.Accent).Render(" ◌ ") +
			lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).Render("Loading lyrics...")
	}
	if l.Error != "" {
		return lipgloss.NewStyle().Foreground(th.Error).Render(" ✗ " + l.Error)
	}
	if !l.Synced && l.PlainText == "" && len(l.Lines) == 0 {
		return lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
			Render(" No lyrics available for this track")
	}

	if l.Synced && len(l.Lines) > 0 {
		return l.viewSynced(th, width, height, positionMs)
	}
	return l.viewPlain(th, width, height)
}

func (l Lyrics) viewSynced(th theme.Theme, width, height int, positionMs int64) string {
	// Find current line index based on playback position
	currentIdx := 0
	for i, line := range l.Lines {
		if line.TimeMs <= positionMs {
			currentIdx = i
		} else {
			break
		}
	}

	// Auto-scroll: center current line vertically
	startIdx := currentIdx - height/2
	if startIdx < 0 {
		startIdx = 0
	}

	accent := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	dim := lipgloss.NewStyle().Foreground(th.FgDim)
	muted := lipgloss.NewStyle().Foreground(th.FgMuted)

	var lines []string
	for i := startIdx; i < len(l.Lines) && len(lines) < height; i++ {
		text := l.Lines[i].Text
		if text == "" {
			text = "♫"
		}
		if len(text) > width-4 {
			text = text[:width-5] + "…"
		}

		if i == currentIdx {
			// Current line: bright, centered with indicator
			lines = append(lines, " ▸ "+accent.Render(text))
		} else if i == currentIdx-1 || i == currentIdx+1 {
			// Adjacent lines: dimmer
			lines = append(lines, "   "+dim.Render(text))
		} else {
			// Far lines: muted
			lines = append(lines, "   "+muted.Render(text))
		}
	}

	return strings.Join(lines, "\n")
}

func (l Lyrics) viewPlain(th theme.Theme, width, height int) string {
	plainLines := strings.Split(l.PlainText, "\n")

	startIdx := l.ScrollPos
	if startIdx >= len(plainLines) {
		startIdx = len(plainLines) - 1
	}
	if startIdx < 0 {
		startIdx = 0
	}

	dim := lipgloss.NewStyle().Foreground(th.FgDim)

	var lines []string
	for i := startIdx; i < len(plainLines) && len(lines) < height; i++ {
		text := plainLines[i]
		if len(text) > width-4 {
			text = text[:width-5] + "…"
		}
		lines = append(lines, "  "+dim.Render(text))
	}

	return strings.Join(lines, "\n")
}
