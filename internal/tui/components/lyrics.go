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

	// Styles. Width(width) makes lipgloss wrap long lines on word boundaries
	// rather than truncating with an ellipsis; Padding keeps text off the
	// panel edges. The active style uses an accent background that paints
	// across every wrapped visual row, so the highlight doesn't break.
	activeStyle := lipgloss.NewStyle().
		Foreground(th.Surface).
		Background(th.Accent).
		Bold(true).
		Width(width).
		Padding(0, 1)
	nearStyle := lipgloss.NewStyle().
		Foreground(th.Fg).
		Width(width).
		Padding(0, 1)
	farStyle := lipgloss.NewStyle().
		Foreground(th.FgMuted).
		Width(width).
		Padding(0, 1)

	renderLine := func(i int) string {
		text := l.Lines[i].Text
		if text == "" {
			text = "♫"
		}
		switch {
		case i == currentIdx:
			return activeStyle.Render("▸ " + text)
		case i == currentIdx-1 || i == currentIdx+1:
			return nearStyle.Render("  " + text)
		default:
			return farStyle.Render("  " + text)
		}
	}

	// Pre-render each logical line so we know its visual height (wrapped
	// lines can occupy 2+ rows). The scroll logic below needs that to
	// pick a startIdx that keeps the active line vertically centered.
	rendered := make([]string, len(l.Lines))
	heights := make([]int, len(l.Lines))
	for i := range l.Lines {
		rendered[i] = renderLine(i)
		heights[i] = lipgloss.Height(rendered[i])
	}

	// Walk backwards from currentIdx until we've accumulated ~height/2
	// rows of context above the active line.
	target := height / 2
	accumulated := 0
	startIdx := currentIdx
	for startIdx > 0 && accumulated+heights[startIdx-1] <= target {
		startIdx--
		accumulated += heights[startIdx]
	}

	// Emit lines until we run out of vertical space. Track visual rows
	// rather than logical lines so wrapped content doesn't overflow.
	var out []string
	total := 0
	for i := startIdx; i < len(l.Lines) && total < height; i++ {
		out = append(out, rendered[i])
		total += heights[i]
	}

	return strings.Join(out, "\n")
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

	dimStyle := lipgloss.NewStyle().
		Foreground(th.FgDim).
		Width(width).
		Padding(0, 1)

	var out []string
	total := 0
	for i := startIdx; i < len(plainLines) && total < height; i++ {
		text := plainLines[i]
		rendered := dimStyle.Render(" " + text)
		out = append(out, rendered)
		total += lipgloss.Height(rendered)
	}

	return strings.Join(out, "\n")
}
