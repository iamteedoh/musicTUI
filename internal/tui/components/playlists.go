package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musictui-go/internal/model"
	"github.com/iamteedoh/musictui-go/internal/theme"
)

type PlaylistMode int

const (
	PlaylistModeList PlaylistMode = iota
	PlaylistModeTracks
)

type Playlists struct {
	Items         []model.Playlist
	Total         uint32
	Selected      int
	Loading       bool
	Error         string
	Mode          PlaylistMode
	CurrentID     string
	CurrentName   string
	Tracks        []model.Track
	TracksTotal   uint32
	TrackSelected int
	TracksLoading bool

	// Set by app to indicate which track is currently playing
	PlayingTrackID string
}

func NewPlaylists() Playlists {
	return Playlists{}
}

func (p *Playlists) NeedsFetch() bool {
	return len(p.Items) == 0 && !p.Loading
}

func (p *Playlists) Up() {
	if p.Mode == PlaylistModeList {
		if p.Selected > 0 {
			p.Selected--
		}
	} else {
		if p.TrackSelected > 0 {
			p.TrackSelected--
		}
	}
}

func (p *Playlists) Down() {
	if p.Mode == PlaylistModeList {
		if p.Selected < len(p.Items)-1 {
			p.Selected++
		}
	} else {
		if p.TrackSelected < len(p.Tracks)-1 {
			p.TrackSelected++
		}
	}
}

func (p *Playlists) Select() (playlistID string, ok bool) {
	if p.Mode == PlaylistModeList && p.Selected < len(p.Items) {
		pl := p.Items[p.Selected]
		p.Mode = PlaylistModeTracks
		p.CurrentID = pl.ID
		p.CurrentName = pl.Name
		p.Tracks = nil
		p.TrackSelected = 0
		p.TracksLoading = true
		return pl.ID, true
	}
	return "", false
}

func (p *Playlists) Back() bool {
	if p.Mode == PlaylistModeTracks {
		p.Mode = PlaylistModeList
		p.Tracks = nil
		return true
	}
	return false
}

func (p *Playlists) SelectedTrack() *model.Track {
	if p.Mode == PlaylistModeTracks && p.TrackSelected < len(p.Tracks) {
		return &p.Tracks[p.TrackSelected]
	}
	return nil
}

func (p Playlists) View(th theme.Theme, width, height int) string {
	if p.Mode == PlaylistModeTracks {
		return p.viewTracks(th, width, height)
	}
	return p.viewList(th, width, height)
}

func (p Playlists) viewList(th theme.Theme, width, height int) string {
	if p.Loading && len(p.Items) == 0 {
		spinner := lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ")
		return " " + spinner + lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).Render("Loading playlists...")
	}
	if len(p.Items) == 0 {
		empty := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("󰲸 No playlists found")
		return lipgloss.NewStyle().
			Width(width).Height(height).
			Align(lipgloss.Center, lipgloss.Center).
			Render(empty)
	}

	var b strings.Builder

	// ── Header ──
	title := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("󰲸  Your Playlists")
	count := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d playlists", p.Total))
	titleWidth := lipgloss.Width(title)
	countWidth := lipgloss.Width(count)
	titleGap := width - titleWidth - countWidth - 2
	if titleGap < 1 {
		titleGap = 1
	}
	b.WriteString(" " + title + strings.Repeat(" ", titleGap) + count + "\n")

	// ── Divider ──
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n")

	// ── Scrollable playlists ──
	visibleRows := height - 3
	if visibleRows < 1 {
		visibleRows = 1
	}
	startIdx := 0
	if p.Selected >= visibleRows {
		startIdx = p.Selected - visibleRows + 1
	}
	endIdx := startIdx + visibleRows
	if endIdx > len(p.Items) {
		endIdx = len(p.Items)
	}

	nameW := width * 50 / 100
	for i := startIdx; i < endIdx; i++ {
		pl := p.Items[i]
		isSelected := i == p.Selected

		name := truncate(pl.Name, nameW)
		info := fmt.Sprintf("%d tracks  ·  %s", pl.TrackCount, pl.Owner)

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸ ")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(name)
			infoStr := lipgloss.NewStyle().Foreground(th.AccentDim).Render("  " + info)
			line := lipgloss.NewStyle().
				Width(width).
				Background(th.SurfaceBright).
				Render(indicator + nameStr + infoStr)
			b.WriteString(line + "\n")
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Render(name)
			infoStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render("  " + info)
			b.WriteString("  " + nameStr + infoStr + "\n")
		}
	}

	// ── Position indicator ──
	if len(p.Items) > visibleRows {
		pos := fmt.Sprintf("%d / %d", p.Selected+1, len(p.Items))
		b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(pos))
	}

	return b.String()
}

func (p Playlists) viewTracks(th theme.Theme, width, height int) string {
	var b strings.Builder

	// ── Back + playlist name header ──
	back := lipgloss.NewStyle().Foreground(th.FgMuted).Render("← ")
	name := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(p.CurrentName)
	count := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("  ·  %d tracks", p.TracksTotal))
	hint := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("  Esc to go back")
	b.WriteString(" " + back + name + count + hint + "\n")

	// ── Divider ──
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n")

	if p.TracksLoading && len(p.Tracks) == 0 {
		spinner := lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ")
		b.WriteString(" " + spinner + lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).Render("Loading tracks..."))
		return b.String()
	}

	// ── Column headers ──
	numW := 5
	nameW := (width - numW - 2) * 40 / 100
	artistW := (width - numW - 2) * 35 / 100

	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	headerRow := " " +
		colHeader.Width(numW).Render("#") +
		colHeader.Width(nameW).Render("TITLE") +
		colHeader.Width(artistW).Render("ARTIST") +
		colHeader.Render("TIME")
	b.WriteString(headerRow + "\n")
	b.WriteString(" " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", width-2)) + "\n")

	// ── Scrollable tracks ──
	visibleRows := height - 5
	if visibleRows < 1 {
		visibleRows = 1
	}
	startIdx := 0
	if p.TrackSelected >= visibleRows {
		startIdx = p.TrackSelected - visibleRows + 1
	}
	endIdx := startIdx + visibleRows
	if endIdx > len(p.Tracks) {
		endIdx = len(p.Tracks)
	}

	for i := startIdx; i < endIdx; i++ {
		t := p.Tracks[i]
		isSelected := i == p.TrackSelected
		isPlaying := p.PlayingTrackID != "" && t.ID == p.PlayingTrackID

		tName := truncate(t.Name, nameW-2)
		artist := truncate(t.ArtistNames(), artistW-2)
		dur := t.FormatDuration()
		num := fmt.Sprintf("%d", i+1)

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			numStr := lipgloss.NewStyle().Foreground(th.Accent).Width(numW - 1).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(nameW).Render(tName)
			artistStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(artistW).Render(artist)
			durStr := lipgloss.NewStyle().Foreground(th.Accent).Render(dur)
			line := lipgloss.NewStyle().
				Width(width).
				Background(th.SurfaceBright).
				Render(indicator + numStr + nameStr + artistStr + durStr)
			b.WriteString(line + "\n")
		} else if isPlaying {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("♫")
			numStr := lipgloss.NewStyle().Foreground(th.Accent).Width(numW - 1).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Width(nameW).Render(tName)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artist)
			durStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(dur)
			b.WriteString(indicator + numStr + nameStr + artistStr + durStr + "\n")
		} else {
			numStr := lipgloss.NewStyle().Foreground(th.FgMuted).Width(numW).Render(num)
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(nameW).Render(tName)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artist)
			durStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(dur)
			b.WriteString(" " + numStr + nameStr + artistStr + durStr + "\n")
		}
	}

	// ── Position indicator ──
	if len(p.Tracks) > visibleRows {
		pos := fmt.Sprintf("%d / %d", p.TrackSelected+1, len(p.Tracks))
		b.WriteString("\n " + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(pos))
	}

	return b.String()
}
