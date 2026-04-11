package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musictui-go/internal/model"
	"github.com/iamteedoh/musictui-go/internal/theme"
)

type Library struct {
	Tracks   []model.Track
	Total    uint32
	Selected int
	Loading  bool
	Offset   int
	Error    string

	// Set by app to indicate which track is currently playing
	PlayingTrackID string
}

func NewLibrary() Library {
	return Library{}
}

func (l *Library) Up() {
	if l.Selected > 0 {
		l.Selected--
	}
}

func (l *Library) Down() {
	if l.Selected < len(l.Tracks)-1 {
		l.Selected++
	}
}

func (l *Library) NeedsFetch() bool {
	return len(l.Tracks) == 0 && !l.Loading
}

func (l *Library) NeedsMore() bool {
	return l.Selected >= len(l.Tracks)-3 && uint32(len(l.Tracks)) < l.Total && !l.Loading
}

func (l *Library) NextOffset() int {
	return len(l.Tracks)
}

func (l *Library) AppendTracks(tracks []model.Track, total uint32, offset uint32) {
	l.Tracks = append(l.Tracks, tracks...)
	l.Total = total
	l.Loading = false
	l.Error = ""
}

func (l *Library) SelectedTrack() *model.Track {
	if l.Selected >= 0 && l.Selected < len(l.Tracks) {
		return &l.Tracks[l.Selected]
	}
	return nil
}

func (l Library) View(th theme.Theme, width, height int) string {
	if l.Loading && len(l.Tracks) == 0 {
		spinner := lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ")
		return "  " + spinner + lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).Render("Loading library...")
	}
	if l.Error != "" {
		return "  " + lipgloss.NewStyle().Foreground(th.Error).Render("✗ "+l.Error)
	}
	if len(l.Tracks) == 0 {
		empty := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("  Your library is empty")
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(empty)
	}

	var b strings.Builder

	// ── Header ──
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("  Liked Songs")
	count := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d tracks", l.Total))
	titleWidth := lipgloss.Width(title)
	countWidth := lipgloss.Width(count)
	titleGap := width - titleWidth - countWidth - 2
	if titleGap < 1 {
		titleGap = 1
	}
	b.WriteString(" " + title + strings.Repeat(" ", titleGap) + count + "\n")

	// ── Divider ──
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n")

	// ── Column headers ──
	numW := 5
	nameW := (width - numW - 2) * 35 / 100
	artistW := (width - numW - 2) * 30 / 100
	albumW := (width - numW - 2) * 25 / 100

	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	headerRow := " " +
		colHeader.Width(numW).Render("#") +
		colHeader.Width(nameW).Render("TITLE") +
		colHeader.Width(artistW).Render("ARTIST") +
		colHeader.Width(albumW).Render("ALBUM") +
		colHeader.Render("TIME")
	b.WriteString(headerRow + "\n")

	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n")

	// ── Scrollable tracks ──
	visibleRows := height - 5 // header(1) + divider(1) + colheaders(1) + divider(1) + buffer
	if visibleRows < 1 {
		visibleRows = 1
	}

	startIdx := 0
	if l.Selected >= visibleRows {
		startIdx = l.Selected - visibleRows + 1
	}
	endIdx := startIdx + visibleRows
	if endIdx > len(l.Tracks) {
		endIdx = len(l.Tracks)
	}

	for i := startIdx; i < endIdx; i++ {
		t := l.Tracks[i]
		isSelected := i == l.Selected
		isPlaying := l.PlayingTrackID != "" && t.ID == l.PlayingTrackID

		name := truncate(t.Name, nameW-2)
		artist := truncate(t.ArtistNames(), artistW-2)
		album := ""
		if t.Album != nil {
			album = truncate(t.Album.Name, albumW-2)
		}
		dur := t.FormatDuration()
		num := fmt.Sprintf("%d", i+1)

		if isSelected {
			// Selected row: accent indicator + full-width highlight
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			numStr := lipgloss.NewStyle().Foreground(th.Accent).Width(numW - 1).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(nameW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(artistW).Render(artist)
			albumStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(albumW).Render(album)
			durStr := lipgloss.NewStyle().Foreground(th.Accent).Render(dur)

			line := lipgloss.NewStyle().
				Width(width).
				Background(th.SurfaceBright).
				Render(indicator + numStr + nameStr + artistStr + albumStr + durStr)
			b.WriteString(line + "\n")
		} else if isPlaying {
			// Playing row: ♫ indicator in accent
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("♫")
			numStr := lipgloss.NewStyle().Foreground(th.Accent).Width(numW - 1).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Width(nameW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artist)
			albumStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(albumW).Render(album)
			durStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(dur)
			b.WriteString(indicator + numStr + nameStr + artistStr + albumStr + durStr + "\n")
		} else {
			// Normal row
			numStr := lipgloss.NewStyle().Foreground(th.FgMuted).Width(numW).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(nameW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artist)
			albumStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(albumW).Render(album)
			durStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(dur)
			b.WriteString(" " + numStr + nameStr + artistStr + albumStr + durStr + "\n")
		}
	}

	// ── Scrollbar indicator ──
	if len(l.Tracks) > visibleRows {
		scrollPos := visibleRows * l.Selected / len(l.Tracks)
		for i := 0; i < visibleRows; i++ {
			_ = scrollPos // we'll render inline instead
		}
		// Compact indicator at bottom
		loaded := fmt.Sprintf("%d / %d", l.Selected+1, len(l.Tracks))
		if l.Loading {
			loaded += " ◌"
		}
		b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(loaded))
	} else if l.Loading {
		b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.Accent).Italic(true).Render("Loading more..."))
	}

	return b.String()
}
