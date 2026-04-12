package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

type Search struct {
	Query          string
	CursorPos      int
	Results        *model.SearchResults
	SelectedResult int
	Loading        bool
	Error          string
	ResultsFocused bool

	// Navigation history for drill-down (artist → albums → tracks)
	history      []model.SearchResults
	historyTitle []string
	StatusTitle  string // current breadcrumb title (e.g., "Albums by Queen")

	// Pagination
	Total  uint32 // total results available on server
	Offset uint32 // how many we've loaded so far
}

func NewSearch() Search {
	return Search{}
}

func (s *Search) InputChar(c rune) {
	before := s.Query[:s.CursorPos]
	after := s.Query[s.CursorPos:]
	s.Query = before + string(c) + after
	s.CursorPos += len(string(c))
}

func (s *Search) Backspace() {
	if s.CursorPos > 0 && len(s.Query) > 0 {
		runes := []rune(s.Query)
		runePos := len([]rune(s.Query[:s.CursorPos]))
		if runePos > 0 {
			newRunes := append(runes[:runePos-1], runes[runePos:]...)
			s.Query = string(newRunes)
			s.CursorPos = len(string(newRunes[:runePos-1]))
		}
	}
}

func (s *Search) Clear() {
	s.Query = ""
	s.CursorPos = 0
	s.Results = nil
	s.SelectedResult = 0
	s.Loading = false
	s.Error = ""
	s.ResultsFocused = false
}

func (s *Search) Submit() bool {
	if strings.TrimSpace(s.Query) != "" {
		s.Loading = true
		s.Error = ""
		s.SelectedResult = 0
		s.ResultsFocused = false
		return true
	}
	return false
}

func (s *Search) SetResults(results model.SearchResults) {
	s.Results = &results
	s.Loading = false
	s.Error = ""
	s.SelectedResult = 0
	s.ResultsFocused = true
}

// AppendResults adds more results from a pagination fetch.
func (s *Search) AppendResults(results model.SearchResults, total, offset uint32) {
	if s.Results == nil {
		s.SetResults(results)
	} else {
		s.Results.Tracks = append(s.Results.Tracks, results.Tracks...)
		s.Results.Albums = append(s.Results.Albums, results.Albums...)
		s.Results.Artists = append(s.Results.Artists, results.Artists...)
		s.Results.Playlists = append(s.Results.Playlists, results.Playlists...)
	}
	s.Total = total
	s.Offset = offset + uint32(len(results.Tracks)+len(results.Albums)+len(results.Artists)+len(results.Playlists))
	s.Loading = false
}

// NeedsMore returns true if the user is near the bottom of results and more are available.
func (s *Search) NeedsMore() bool {
	if s.Loading || s.Results == nil {
		return false
	}
	total := s.TotalItems()
	return s.SelectedResult >= total-3 && uint32(total) < s.Total
}

// NextOffset returns the offset for the next page fetch.
func (s *Search) NextOffset() int {
	return s.TotalItems()
}

// PushResults saves current results to history stack before showing new results.
func (s *Search) PushResults(newResults model.SearchResults, title string) {
	if s.Results != nil {
		s.history = append(s.history, *s.Results)
		s.historyTitle = append(s.historyTitle, s.StatusTitle)
	}
	s.StatusTitle = title
	s.SetResults(newResults)
}

// PopResults restores the previous results from the history stack.
// Returns false if the stack is empty.
func (s *Search) PopResults() bool {
	if len(s.history) == 0 {
		return false
	}
	last := len(s.history) - 1
	s.Results = &s.history[last]
	s.StatusTitle = s.historyTitle[last]
	s.history = s.history[:last]
	s.historyTitle = s.historyTitle[:last]
	s.SelectedResult = 0
	s.ResultsFocused = true
	return true
}

func (s *Search) SetError(err string) {
	s.Loading = false
	s.Error = err
}

func (s *Search) TotalItems() int {
	if s.Results == nil {
		return 0
	}
	return len(s.Results.Tracks) + len(s.Results.Artists) + len(s.Results.Albums) + len(s.Results.Playlists)
}

func (s *Search) Up()   { if s.SelectedResult > 0 { s.SelectedResult-- } }
func (s *Search) Down() {
	total := s.TotalItems()
	if total > 0 && s.SelectedResult < total-1 {
		s.SelectedResult++
	}
}

func (s *Search) SelectedItem() (kind string, item interface{}) {
	if s.Results == nil {
		return "", nil
	}
	idx := s.SelectedResult

	// Order: Primary → Others
	switch s.Results.PrimaryType {
	case "artist":
		if idx < len(s.Results.Artists) {
			return "artist", s.Results.Artists[idx]
		}
		idx -= len(s.Results.Artists)
		if idx < len(s.Results.Albums) {
			return "album", s.Results.Albums[idx]
		}
		idx -= len(s.Results.Albums)
		if idx < len(s.Results.Tracks) {
			return "track", s.Results.Tracks[idx]
		}
		idx -= len(s.Results.Tracks)
	case "album":
		if idx < len(s.Results.Albums) {
			return "album", s.Results.Albums[idx]
		}
		idx -= len(s.Results.Albums)
		if idx < len(s.Results.Artists) {
			return "artist", s.Results.Artists[idx]
		}
		idx -= len(s.Results.Artists)
		if idx < len(s.Results.Tracks) {
			return "track", s.Results.Tracks[idx]
		}
		idx -= len(s.Results.Tracks)
	default: // track
		if idx < len(s.Results.Tracks) {
			return "track", s.Results.Tracks[idx]
		}
		idx -= len(s.Results.Tracks)
		if idx < len(s.Results.Artists) {
			return "artist", s.Results.Artists[idx]
		}
		idx -= len(s.Results.Artists)
		if idx < len(s.Results.Albums) {
			return "album", s.Results.Albums[idx]
		}
		idx -= len(s.Results.Albums)
	}

	if idx < len(s.Results.Playlists) {
		return "playlist", s.Results.Playlists[idx]
	}
	return "", nil
}

func (s Search) View(th theme.Theme, width, height int) string {
	var sections []string

	// ── Search Input ──
	inputBorderColor := th.Border
	if !s.ResultsFocused {
		inputBorderColor = th.Accent
	}

	var inputContent string
	searchIcon := lipgloss.NewStyle().Foreground(th.FgMuted).Render("  ")

	if s.Query == "" && !s.ResultsFocused {
		ms := time.Now().UnixMilli()
		if (ms/530)%2 == 0 {
			cursor := lipgloss.NewStyle().Foreground(th.Bg).Background(th.Fg).Render(" ")
			inputContent = searchIcon + cursor
		} else {
			inputContent = searchIcon + lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Search tracks, artists, albums...")
		}
	} else if s.ResultsFocused {
		inputContent = searchIcon + lipgloss.NewStyle().Foreground(th.FgMuted).Render(s.Query)
	} else {
		before := s.Query[:s.CursorPos]
		after := s.Query[s.CursorPos:]
		cursorChar := " "
		remaining := after
		if len(after) > 0 {
			runes := []rune(after)
			cursorChar = string(runes[0])
			remaining = string(runes[1:])
		}
		ms := time.Now().UnixMilli()
		cursorStyle := lipgloss.NewStyle().Foreground(th.Fg)
		if (ms/530)%2 == 0 {
			cursorStyle = lipgloss.NewStyle().Foreground(th.Bg).Background(th.Fg)
		}
		inputContent = searchIcon +
			lipgloss.NewStyle().Foreground(th.Fg).Render(before) +
			cursorStyle.Render(cursorChar) +
			lipgloss.NewStyle().Foreground(th.Fg).Render(remaining)
	}

	inputBox := lipgloss.NewStyle().
		Width(width - 4).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorderColor).
		Render(inputContent)

	sections = append(sections, inputBox)

	// ── Status ──
	if s.Loading {
		sections = append(sections, " "+lipgloss.NewStyle().Foreground(th.Accent).Render("◌ ")+
			lipgloss.NewStyle().Foreground(th.FgDim).Italic(true).Render(fmt.Sprintf("Searching for \"%s\"...", s.Query)))
		return strings.Join(sections, "\n")
	}
	if s.Error != "" {
		sections = append(sections, " "+lipgloss.NewStyle().Foreground(th.Error).Render("✗ "+s.Error))
		return strings.Join(sections, "\n")
	}
	if s.Results == nil {
		sections = append(sections, " "+lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("Type a query and press Enter"))
		return strings.Join(sections, "\n")
	}
	if s.TotalItems() == 0 {
		sections = append(sections, " "+lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("No results for \"%s\"", s.Query)))
		return strings.Join(sections, "\n")
	}

	// Show breadcrumb/title when in a drill-down (e.g. "Tracks from Album Name")
	if s.StatusTitle != "" && len(s.history) > 0 {
		breadcrumb := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(" ← " + s.StatusTitle)
		hint := lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("  (Esc to go back)")
		sections = append(sections, breadcrumb+hint)
	}

	// ── Result cards ──
	cardWidth := width - 4
	itemIdx := 0

	// Order cards based on PrimaryType
	renderTracks := func() {
		if len(s.Results.Tracks) > 0 {
			sections = append(sections, renderTrackCard(th, s.Results.Tracks, &itemIdx, s.SelectedResult, cardWidth))
		}
	}
	renderArtists := func() {
		if len(s.Results.Artists) > 0 {
			sections = append(sections, renderArtistCard(th, s.Results.Artists, &itemIdx, s.SelectedResult, cardWidth))
		}
	}
	renderAlbums := func() {
		if len(s.Results.Albums) > 0 {
			sections = append(sections, renderAlbumCard(th, s.Results.Albums, &itemIdx, s.SelectedResult, cardWidth))
		}
	}

	switch s.Results.PrimaryType {
	case "artist":
		renderArtists()
		renderAlbums()
		renderTracks()
	case "album":
		renderAlbums()
		renderArtists()
		renderTracks()
	default: // track
		renderTracks()
		renderArtists()
		renderAlbums()
	}

	if len(s.Results.Playlists) > 0 {
		sections = append(sections, renderPlaylistCard(th, s.Results.Playlists, &itemIdx, s.SelectedResult, cardWidth))
	}

	return strings.Join(sections, "\n")
}

func renderTrackCard(th theme.Theme, tracks []model.Track, itemIdx *int, selected int, cardWidth int) string {
	innerWidth := cardWidth - 2

	// Column widths
	titleW := innerWidth * 45 / 100
	artistW := innerWidth * 38 / 100
	timeW := innerWidth - titleW - artistW

	// Header
	titleStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("  Tracks")
	countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d results ", len(tracks)))
	headerGap := innerWidth - lipgloss.Width(titleStr) - lipgloss.Width(countStr)
	if headerGap < 1 {
		headerGap = 1
	}
	header := titleStr + strings.Repeat(" ", headerGap) + countStr

	// Column headers
	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	colLine := " " +
		colHeader.Width(titleW).Render("TITLE") +
		colHeader.Width(artistW).Render("ARTIST") +
		colHeader.Width(timeW).Render("TIME")
	divider := " " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", innerWidth-2))

	// Rows
	var rows []string
	for _, t := range tracks {
		isSelected := *itemIdx == selected

		name := truncate(t.Name, titleW-2)
		artist := truncate(t.ArtistNames(), artistW-2)
		dur := t.FormatDuration()

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(titleW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(artistW).Render(artist)
			durStr := lipgloss.NewStyle().Foreground(th.Accent).Width(timeW).Render(dur)
			row := lipgloss.NewStyle().
				Width(innerWidth).
				Background(th.SurfaceBright).
				Render(indicator + nameStr + artistStr + durStr)
			rows = append(rows, row)
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(titleW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artist)
			durStr := lipgloss.NewStyle().Foreground(th.FgMuted).Width(timeW).Render(dur)
			rows = append(rows, " "+nameStr+artistStr+durStr)
		}
		*itemIdx++
	}

	content := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Width(cardWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		PaddingLeft(1).PaddingRight(1).
		Render(header + "\n" + colLine + "\n" + divider + "\n" + content)
}

func renderAlbumCard(th theme.Theme, albums []model.Album, itemIdx *int, selected int, cardWidth int) string {
	innerWidth := cardWidth - 2

	albumW := innerWidth * 55 / 100
	artistW := innerWidth * 30 / 100
	yearW := innerWidth - albumW - artistW

	titleStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("  Albums")
	countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d results ", len(albums)))
	headerGap := innerWidth - lipgloss.Width(titleStr) - lipgloss.Width(countStr)
	if headerGap < 1 {
		headerGap = 1
	}
	header := titleStr + strings.Repeat(" ", headerGap) + countStr

	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	colLine := " " +
		colHeader.Width(albumW).Render("ALBUM") +
		colHeader.Width(artistW).Render("ARTIST") +
		colHeader.Width(yearW).Render("YEAR")
	divider := " " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", innerWidth-2))

	var rows []string
	for _, a := range albums {
		isSelected := *itemIdx == selected
		name := truncate(a.Name, albumW-2)
		artists := ""
		for i, art := range a.Artists {
			if i > 0 {
				artists += ", "
			}
			artists += art.Name
		}
		artists = truncate(artists, artistW-2)
		year := ""
		if len(a.ReleaseDate) >= 4 {
			year = a.ReleaseDate[:4]
		}

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(albumW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(artistW).Render(artists)
			yearStr := lipgloss.NewStyle().Foreground(th.Accent).Width(yearW).Render(year)
			row := lipgloss.NewStyle().Width(innerWidth).Background(th.SurfaceBright).
				Render(indicator + nameStr + artistStr + yearStr)
			rows = append(rows, row)
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(albumW).Render(name)
			artistStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(artistW).Render(artists)
			yearStr := lipgloss.NewStyle().Foreground(th.FgMuted).Width(yearW).Render(year)
			rows = append(rows, " "+nameStr+artistStr+yearStr)
		}
		*itemIdx++
	}

	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(cardWidth).
		Border(lipgloss.RoundedBorder()).BorderForeground(th.Border).
		PaddingLeft(1).PaddingRight(1).
		Render(header + "\n" + colLine + "\n" + divider + "\n" + content)
}

func renderArtistCard(th theme.Theme, artists []model.Artist, itemIdx *int, selected int, cardWidth int) string {
	innerWidth := cardWidth - 2

	titleStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render("  Artists")
	countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d results ", len(artists)))
	headerGap := innerWidth - lipgloss.Width(titleStr) - lipgloss.Width(countStr)
	if headerGap < 1 {
		headerGap = 1
	}
	header := titleStr + strings.Repeat(" ", headerGap) + countStr

	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	colLine := " " + colHeader.Width(innerWidth-2).Render("ARTIST")
	divider := " " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", innerWidth-2))

	var rows []string
	for _, a := range artists {
		isSelected := *itemIdx == selected
		name := truncate(a.Name, innerWidth-4)

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(innerWidth - 1).Render(name)
			row := lipgloss.NewStyle().Width(innerWidth).Background(th.SurfaceBright).
				Render(indicator + nameStr)
			rows = append(rows, row)
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(innerWidth - 2).Render(name)
			rows = append(rows, " "+nameStr)
		}
		*itemIdx++
	}

	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(cardWidth).
		Border(lipgloss.RoundedBorder()).BorderForeground(th.Border).
		PaddingLeft(1).PaddingRight(1).
		Render(header + "\n" + colLine + "\n" + divider + "\n" + content)
}

func renderPlaylistCard(th theme.Theme, playlists []model.Playlist, itemIdx *int, selected int, cardWidth int) string {
	innerWidth := cardWidth - 2

	nameW := innerWidth * 55 / 100
	ownerW := innerWidth * 25 / 100
	countW := innerWidth - nameW - ownerW

	titleStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(" 󰲸 Playlists")
	countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d results ", len(playlists)))
	headerGap := innerWidth - lipgloss.Width(titleStr) - lipgloss.Width(countStr)
	if headerGap < 1 {
		headerGap = 1
	}
	header := titleStr + strings.Repeat(" ", headerGap) + countStr

	colHeader := lipgloss.NewStyle().Foreground(th.FgMuted).Bold(true)
	colLine := " " +
		colHeader.Width(nameW).Render("NAME") +
		colHeader.Width(ownerW).Render("OWNER") +
		colHeader.Width(countW).Render("TRACKS")
	divider := " " + lipgloss.NewStyle().Foreground(th.Border).Render(strings.Repeat("─", innerWidth-2))

	var rows []string
	for _, p := range playlists {
		isSelected := *itemIdx == selected
		name := truncate(p.Name, nameW-2)
		owner := truncate(p.Owner, ownerW-2)
		tracks := fmt.Sprintf("%d", p.TrackCount)

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸")
			nameStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Width(nameW).Render(name)
			ownerStr := lipgloss.NewStyle().Foreground(th.AccentDim).Width(ownerW).Render(owner)
			countStr := lipgloss.NewStyle().Foreground(th.Accent).Width(countW).Render(tracks)
			row := lipgloss.NewStyle().Width(innerWidth).Background(th.SurfaceBright).
				Render(indicator + nameStr + ownerStr + countStr)
			rows = append(rows, row)
		} else {
			nameStr := lipgloss.NewStyle().Foreground(th.Fg).Width(nameW).Render(name)
			ownerStr := lipgloss.NewStyle().Foreground(th.FgDim).Width(ownerW).Render(owner)
			countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Width(countW).Render(tracks)
			rows = append(rows, " "+nameStr+ownerStr+countStr)
		}
		*itemIdx++
	}

	content := strings.Join(rows, "\n")
	return lipgloss.NewStyle().Width(cardWidth).
		Border(lipgloss.RoundedBorder()).BorderForeground(th.Border).
		PaddingLeft(1).PaddingRight(1).
		Render(header + "\n" + colLine + "\n" + divider + "\n" + content)
}

func renderResultCard[T any](th theme.Theme, title string, items []T, itemIdx *int, selected int, cardWidth int, format func(T) (string, string)) string {
	innerWidth := cardWidth - 2 // padding only (borders are added outside Width)

	// Card header: icon+title on left, count on right
	titleStr := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(" " + title)
	countStr := lipgloss.NewStyle().Foreground(th.FgMuted).Render(fmt.Sprintf("%d results ", len(items)))

	// Build rows
	var rows []string
	for _, item := range items {
		primary, secondary := format(item)
		isSelected := *itemIdx == selected

		if isSelected {
			indicator := lipgloss.NewStyle().Foreground(th.Accent).Render("▸ ")
			name := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
				Render(truncate(primary, innerWidth/2))
			detail := ""
			if secondary != "" {
				detail = "  " + lipgloss.NewStyle().Foreground(th.AccentDim).
					Render(truncate(secondary, innerWidth/3))
			}
			row := lipgloss.NewStyle().
				Width(innerWidth).
				Background(th.SurfaceBright).
				Render(indicator + name + detail)
			rows = append(rows, row)
		} else {
			name := lipgloss.NewStyle().Foreground(th.Fg).
				Render(truncate(primary, innerWidth/2))
			detail := ""
			if secondary != "" {
				detail = "  " + lipgloss.NewStyle().Foreground(th.FgMuted).
					Render(truncate(secondary, innerWidth/3))
			}
			rows = append(rows, "  "+name+detail)
		}
		*itemIdx++
	}

	content := strings.Join(rows, "\n")

	// Header line with title and count
	headerGap := innerWidth - lipgloss.Width(titleStr) - lipgloss.Width(countStr)
	if headerGap < 1 {
		headerGap = 1
	}
	header := titleStr + strings.Repeat(" ", headerGap) + countStr

	return lipgloss.NewStyle().
		Width(cardWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(th.Border).
		PaddingLeft(1).PaddingRight(1).
		Render(header + "\n" + content)
}
