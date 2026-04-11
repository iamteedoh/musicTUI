package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	spotifylib "github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"

	"github.com/iamteedoh/musictui-go/internal/audio"
	"github.com/iamteedoh/musictui-go/internal/config"
	"github.com/iamteedoh/musictui-go/internal/model"
	sp "github.com/iamteedoh/musictui-go/internal/spotify"
	"github.com/iamteedoh/musictui-go/internal/theme"
	"github.com/iamteedoh/musictui-go/internal/tui/components"
)

type App struct {
	config   config.Config
	theme    theme.Theme
	width    int
	height   int
	focus      model.FocusMode
	view       model.View
	showLyrics bool // toggle inline lyrics in center panel (default: true)
	sidebar  components.Sidebar
	home     components.Home
	library  components.Library
	search   components.Search
	playlist components.Playlists
	playing  components.NowPlaying
	viz      components.MiniVisualizer
	lyrics   components.Lyrics
	artwork  components.Artwork
	playback model.PlaybackState
	queue    model.Queue
	status   string

	// Spotify
	auth        *sp.Auth
	client      *sp.Client
	accessToken string

	// Audio
	engine     *audio.Engine
	bridgePath string
}

func NewApp(cfg config.Config, bridgePath string) App {
	th := theme.FromName(cfg.Theme)
	app := App{
		config:   cfg,
		theme:    th,
		focus:    model.FocusSidebar,
		view:     model.ViewHome,
		sidebar:  components.NewSidebar(),
		home:     components.NewHome(),
		library:  components.NewLibrary(),
		search:   components.NewSearch(),
		playlist: components.NewPlaylists(),
		playing:  components.NewNowPlaying(),
		viz:      components.NewMiniVisualizer(),
		lyrics:   components.NewLyrics(),
		artwork:  components.NewArtwork(),
		showLyrics: true,
		playback:   model.PlaybackState{Volume: cfg.Volume},
		queue:      model.NewQueue(),
		bridgePath: bridgePath,
	}
	return app
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}

	// Try cached auth on startup
	if a.config.Spotify.ClientID != "" {
		cmds = append(cmds, a.tryCachedAuthCmd())
	}

	return tea.Batch(cmds...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

func (a App) tryCachedAuthCmd() tea.Cmd {
	return func() tea.Msg {
		tok, err := sp.CachedToken()
		if err != nil {
			return StatusMsg("No cached credentials — press Ctrl+L to authenticate")
		}
		if tok.RefreshToken == "" && !tok.Valid() {
			return StatusMsg("Token expired with no refresh token — press Ctrl+L to re-authenticate")
		}

		auth := sp.NewAuth(a.config.Spotify.ClientID)
		httpClient := auth.HTTPClient(tok)

		// The httpClient auto-refreshes via oauth2 transport.
		// Make a test call to trigger refresh if needed.
		client := sp.NewClient(spotifylib.New(httpClient))
		username, err := client.FetchUsername(context.Background())
		if err != nil {
			return AuthErrorMsg{Err: fmt.Errorf("auth failed (try Ctrl+L to re-authenticate): %w", err)}
		}

		// Save the potentially refreshed token and grab access token for librespot
		accessToken := tok.AccessToken
		if transport, ok := httpClient.Transport.(*oauth2.Transport); ok {
			if newTok, err := transport.Source.Token(); err == nil {
				_ = sp.SaveToken(newTok)
				accessToken = newTok.AccessToken
			}
		}

		return AuthSuccessMsg{Client: client, Username: username, AccessToken: accessToken}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case TickMsg:
		a.viz.SetPosition(a.playback.Position.Milliseconds())
		a.viz.Update(a.playback.IsPlaying)
		return a, tickCmd()

	// Auth
	case AuthSuccessMsg:
		a.client = msg.Client
		a.home.Username = msg.Username
		a.accessToken = msg.AccessToken
		a.status = "" // Auth info shown in home view and title bar

		// Create audio engine (lazy — bridge subprocess starts on first play)
		if a.bridgePath != "" && a.engine == nil {
			a.engine = audio.NewEngine(a.bridgePath, msg.AccessToken)
			a.viz.SetSpectrum(a.engine.Spectrum)
		}

		// Auto-load playlists for the left panel
		return a, FetchPlaylistsCmd(msg.Client, 0)
	case AuthErrorMsg:
		a.status = "Auth error: " + msg.Err.Error()
		return a, nil

	// Data loaded
	case LibraryLoadedMsg:
		a.library.AppendTracks(msg.Tracks, msg.Total, msg.Offset)
		return a, nil
	case PlaylistsLoadedMsg:
		a.playlist.Items = append(a.playlist.Items, msg.Playlists...)
		a.playlist.Total = msg.Total
		a.playlist.Loading = false
		return a, nil
	case PlaylistTracksLoadedMsg:
		a.playlist.Tracks = append(a.playlist.Tracks, msg.Tracks...)
		a.playlist.TracksTotal = msg.Total
		a.playlist.TracksLoading = false
		return a, nil
	case SearchLoadedMsg:
		if msg.Append {
			a.search.AppendResults(msg.Results, msg.Total, msg.Offset)
		} else {
			a.search.SetResults(msg.Results)
			a.search.Total = msg.Total
			a.search.Offset = uint32(a.search.TotalItems())
		}
		a.focus = model.FocusContent
		return a, nil
	case ArtistAlbumsLoadedMsg:
		results := model.SearchResults{Albums: msg.Albums}
		title := fmt.Sprintf("Albums by %s", msg.Artist.Name)
		a.search.PushResults(results, title)
		a.focus = model.FocusContent
		a.status = title
		return a, nil
	case AlbumTracksLoadedMsg:
		title := fmt.Sprintf("Tracks from %s", msg.Album.Name)
		// Attach album info to each track (SimpleTrack doesn't include it)
		albumRef := &model.AlbumRef{
			ID:       msg.Album.ID,
			Name:     msg.Album.Name,
			ImageURL: msg.Album.ImageURL,
		}
		for i := range msg.Tracks {
			if msg.Tracks[i].Album == nil {
				msg.Tracks[i].Album = albumRef
			}
		}
		if msg.Offset == 0 {
			results := model.SearchResults{Tracks: msg.Tracks}
			a.search.PushResults(results, title)
		} else {
			a.search.AppendResults(model.SearchResults{Tracks: msg.Tracks}, msg.Total, msg.Offset)
		}
		a.focus = model.FocusContent
		a.status = title
		// Auto-fetch more if there are remaining tracks
		if uint32(len(msg.Tracks))+msg.Offset < msg.Total && a.client != nil {
			nextOffset := int(msg.Offset) + len(msg.Tracks)
			return a, FetchAlbumTracksCmd(a.client, msg.Album, nextOffset)
		}
		return a, nil
	case LyricsLoadedMsg:
		a.lyrics.SetLyrics(msg.Result, msg.TrackID)
		return a, nil
	case ArtworkLoadedMsg:
		if msg.Result.Err != "" {
			a.artwork.SetError(msg.Result.Err)
		} else if msg.Result.Img != nil {
			a.artwork.SetFullImage(msg.Result.Img, msg.Result.URL)
		}
		return a, nil
	case DataErrorMsg:
		a.status = "Error: " + msg.Err.Error()
		a.library.Loading = false
		a.playlist.Loading = false
		a.playlist.TracksLoading = false
		a.search.Loading = false
		return a, nil
	case StatusMsg:
		a.status = string(msg)
		return a, nil

	// Playback
	case PlayQueueMsg:
		a.queue.SetQueue(msg.Tracks, msg.StartIdx)
		if t := a.queue.Current(); t != nil {
			return a, func() tea.Msg { return PlayTrackMsg{Track: *t} }
		}
		return a, nil

	case PlayTrackMsg:
		if a.engine != nil {
			a.playback.Track = &msg.Track
			a.playback.IsPlaying = false // Wait for "playing" event from bridge
			a.playback.Position = 0
			a.library.PlayingTrackID = msg.Track.ID
			a.playlist.PlayingTrackID = msg.Track.ID
			a.status = "Loading track..."

			// Fetch lyrics for the new track
			durationSec := int(msg.Track.Duration.Seconds())
			lyricCmd := FetchLyricsCmd(msg.Track.Name, msg.Track.ArtistNames(), durationSec, msg.Track.ID)
			a.lyrics.Loading = true
			a.lyrics.TrackID = msg.Track.ID

			// Load album artwork + info
			albumName := ""
			if msg.Track.Album != nil {
				albumName = msg.Track.Album.Name
				if msg.Track.Album.ImageURL != "" {
					a.artwork.LoadURL(msg.Track.Album.ImageURL)
				}
			}
			a.artwork.SetAlbumInfo(albumName, msg.Track.ArtistNames())

			var artCmd tea.Cmd
			if msg.Track.Album != nil && msg.Track.Album.ImageURL != "" {
				artCmd = FetchArtworkCmd(msg.Track.Album.ImageURL)
			}

			wasStarted := a.engine.Started()
			if err := a.engine.PlayTrack(msg.Track.ID); err != nil {
				a.status = "Play error: " + err.Error()
				return a, nil
			}
			cmds := []tea.Cmd{lyricCmd}
			if artCmd != nil {
				cmds = append(cmds, artCmd)
			}
			if !wasStarted {
				cmds = append(cmds, ListenForAudioEvents(a.engine))
			}
			return a, tea.Batch(cmds...)
		} else {
			a.status = "No audio engine available"
		}
		return a, nil

	case AudioEventMsg:
		switch msg.Event.Kind {
		case "playing":
			a.playback.IsPlaying = true
			a.playback.Position = time.Duration(msg.Event.PositionMs) * time.Millisecond
			a.status = "" // clear "Loading track..."
		case "paused":
			a.playback.IsPlaying = false
			a.playback.Position = time.Duration(msg.Event.PositionMs) * time.Millisecond
		case "position":
			a.playback.Position = time.Duration(msg.Event.PositionMs) * time.Millisecond
		case "end_of_track":
			a.playback.IsPlaying = false
			// Auto-advance to next track in queue
			if next := a.queue.Next(); next != nil {
				if a.engine != nil {
					return a, tea.Batch(
						ListenForAudioEvents(a.engine),
						func() tea.Msg { return PlayTrackMsg{Track: *next} },
					)
				}
			}
		case "stopped":
			a.playback.IsPlaying = false
		case "error":
			a.status = "Playback error: " + msg.Event.Error
		case "loading":
			a.status = "Loading track..."
		}
		if a.engine != nil {
			return a, ListenForAudioEvents(a.engine)
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit
	case "q":
		if a.view != model.ViewSearch || !a.isSearchInputFocused() {
			return a, tea.Quit
		}
	case "tab":
		// In search with results: Tab toggles input/results focus
		if a.view == model.ViewSearch && a.search.Results != nil && a.focus == model.FocusContent {
			a.search.ResultsFocused = !a.search.ResultsFocused
			return a, nil
		}
		// Cycle: Sidebar → Content → Right → Sidebar
		switch a.focus {
		case model.FocusSidebar:
			a.focus = model.FocusContent
		case model.FocusContent:
			a.focus = model.FocusRight
		case model.FocusRight:
			a.focus = model.FocusSidebar
		}
		return a, nil
	case "shift+tab", "backtab":
		// Reverse cycle: Sidebar ← Content ← Right
		switch a.focus {
		case model.FocusSidebar:
			a.focus = model.FocusRight
		case model.FocusContent:
			a.focus = model.FocusSidebar
		case model.FocusRight:
			a.focus = model.FocusContent
		}
		return a, nil
	case "/":
		if a.view != model.ViewSearch || a.search.ResultsFocused {
			a.view = model.ViewSearch
			a.focus = model.FocusContent
			a.search.ResultsFocused = false
			// Update sidebar selection to match
			for i, item := range a.sidebar.Items {
				if item.View == model.ViewSearch {
					a.sidebar.Selected = i
					break
				}
			}
			return a, nil
		}
	}

	// Arrow keys for panel switching (unless typing in search)
	if !a.isSearchInputFocused() {
		switch msg.String() {
		case "left":
			switch a.focus {
			case model.FocusContent:
				a.focus = model.FocusSidebar
				return a, nil
			case model.FocusRight:
				a.focus = model.FocusContent
				return a, nil
			}
		case "right":
			switch a.focus {
			case model.FocusSidebar:
				a.focus = model.FocusContent
				return a, nil
			case model.FocusContent:
				a.focus = model.FocusRight
				return a, nil
			}
		}
	}

	// Playback keys (unless typing in search)
	if !a.isSearchInputFocused() {
		switch msg.String() {
		case " ":
			if a.engine != nil {
				if a.playback.IsPlaying {
					_ = a.engine.Pause()
				} else {
					_ = a.engine.Resume()
				}
			}
			return a, nil
		case "+", "=":
			if a.engine != nil {
				vol := a.engine.Volume() + 5
				_ = a.engine.SetVolume(vol)
				a.playback.Volume = a.engine.Volume()
			}
			return a, nil
		case "-":
			if a.engine != nil {
				vol := a.engine.Volume() - 5
				_ = a.engine.SetVolume(vol)
				a.playback.Volume = a.engine.Volume()
			}
			return a, nil
		case "n":
			if next := a.queue.Next(); next != nil {
				return a, func() tea.Msg { return PlayTrackMsg{Track: *next} }
			}
			return a, nil
		case "p":
			if prev := a.queue.Previous(); prev != nil {
				return a, func() tea.Msg { return PlayTrackMsg{Track: *prev} }
			}
			return a, nil
		case "s":
			a.queue.Shuffle = !a.queue.Shuffle
			a.playback.Shuffle = a.queue.Shuffle
			return a, nil
		case "r":
			a.queue.Repeat = a.queue.Repeat.Next()
			a.playback.Repeat = a.queue.Repeat
			return a, nil
		case "l":
			a.showLyrics = !a.showLyrics
			return a, nil
		}
	}

	switch a.focus {
	case model.FocusSidebar:
		return a.handleNavKey(msg)
	case model.FocusRight:
		return a.handleRightKey(msg)
	default:
		return a.handleContentKey(msg)
	}
}

func (a App) isSearchInputFocused() bool {
	return a.view == model.ViewSearch && !a.search.ResultsFocused
}

func (a App) handleNavKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		a.sidebar.Down()
		a.view = a.sidebar.CurrentView()
		return a, a.onViewEnter()
	case "k", "up":
		a.sidebar.Up()
		a.view = a.sidebar.CurrentView()
		return a, a.onViewEnter()
	case "enter", "l":
		a.view = a.sidebar.CurrentView()
		a.focus = model.FocusContent
		return a, a.onViewEnter()
	}
	return a, nil
}

// onViewEnter triggers data fetching when navigating to a view.
func (a App) onViewEnter() tea.Cmd {
	if a.client == nil {
		return nil
	}
	switch a.view {
	case model.ViewLibrary:
		if a.library.NeedsFetch() {
			a.library.Loading = true
			return FetchLibraryCmd(a.client, 0)
		}
	case model.ViewPlaylists:
		if a.playlist.NeedsFetch() {
			a.playlist.Loading = true
			return FetchPlaylistsCmd(a.client, 0)
		}
	}
	return nil
}

func (a App) handleContentKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.view {
	case model.ViewSearch:
		return a.handleSearchKey(msg)
	case model.ViewLibrary:
		return a.handleLibraryKey(msg)
	case model.ViewPlaylists:
		return a.handlePlaylistKey(msg)
	case model.ViewLyrics:
		switch msg.String() {
		case "j", "down":
			a.lyrics.ScrollDown()
		case "k", "up":
			a.lyrics.ScrollUp()
		case "esc", "h":
			a.focus = model.FocusSidebar
		}
	default:
		switch msg.String() {
		case "esc", "h":
			a.focus = model.FocusSidebar
		}
	}
	return a, nil
}

func (a App) handleLibraryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		a.library.Down()
		if a.library.NeedsMore() && a.client != nil {
			a.library.Loading = true
			return a, FetchLibraryCmd(a.client, a.library.NextOffset())
		}
	case "k", "up":
		a.library.Up()
	case "enter":
		if t := a.library.SelectedTrack(); t != nil {
			idx := a.library.Selected
			tracks := a.library.Tracks
			return a, func() tea.Msg { return PlayQueueMsg{Tracks: tracks, StartIdx: idx} }
		}
	case "esc", "h":
		a.focus = model.FocusSidebar
	}
	return a, nil
}

func (a App) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.search.ResultsFocused {
		key := msg.String()
		switch key {
		case "j", "down":
			a.search.Down()
			// Auto-fetch more results when near the bottom
			if a.search.NeedsMore() && a.client != nil {
				a.search.Loading = true
				return a, SearchMoreCmd(a.client, a.search.Query, a.search.NextOffset())
			}
		case "k", "up":
			a.search.Up()
		case "enter":
			kind, item := a.search.SelectedItem()
			switch kind {
			case "track":
				if _, ok := item.(model.Track); ok && a.search.Results != nil {
					// Queue all tracks in the results, start at the selected one
					tracks := a.search.Results.Tracks
					idx := a.search.SelectedResult // index within tracks section
					return a, func() tea.Msg { return PlayQueueMsg{Tracks: tracks, StartIdx: idx} }
				}
			case "artist":
				if art, ok := item.(model.Artist); ok && a.client != nil {
					a.status = "Albums by " + art.Name
					return a, FetchArtistAlbumsCmd(a.client, art, 0)
				}
			case "album":
				if alb, ok := item.(model.Album); ok && a.client != nil {
					a.status = "Tracks from " + alb.Name
					return a, FetchAlbumTracksCmd(a.client, alb, 0)
				}
			case "playlist":
				a.status = "Playlist drill-down coming soon"
			}
		case "esc":
			// Try to go back in search history before exiting
			if a.search.PopResults() {
				a.status = a.search.StatusTitle
				return a, nil
			}
			a.focus = model.FocusSidebar
		}
		return a, nil
	}

	// Input mode
	switch msg.String() {
	case "backspace":
		a.search.Backspace()
	case "enter":
		if a.search.Submit() && a.client != nil {
			return a, SearchCmd(a.client, a.search.Query)
		}
	case "esc":
		if a.search.Query == "" {
			a.focus = model.FocusSidebar
		} else {
			a.search.Clear()
		}
	default:
		// Any printable character (including space)
		s := msg.String()
		if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
			a.search.InputChar(rune(s[0]))
		} else if len(msg.Runes) > 0 {
			for _, r := range msg.Runes {
				a.search.InputChar(r)
			}
		}
	}
	return a, nil
}

func (a App) handlePlaylistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		a.playlist.Down()
	case "k", "up":
		a.playlist.Up()
	case "enter":
		if id, ok := a.playlist.Select(); ok && a.client != nil {
			return a, FetchPlaylistTracksCmd(a.client, id, 0)
		}
	case "esc", "h":
		if !a.playlist.Back() {
			a.focus = model.FocusSidebar
		}
	}
	return a, nil
}

// handleRightKey handles keys when the TRACKLIST panel is focused.
func (a App) handleRightKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if !a.queue.IsEmpty() && a.queue.Index < a.queue.Len()-1 {
			a.queue.Index++
		}
	case "k", "up":
		if a.queue.Index > 0 {
			a.queue.Index--
		}
	case "enter":
		if t := a.queue.Current(); t != nil {
			return a, func() tea.Msg { return PlayTrackMsg{Track: *t} }
		}
	case "esc", "h":
		a.focus = model.FocusSidebar
	}
	return a, nil
}

func (a App) viewTitle() string {
	switch a.view {
	case model.ViewHome:
		return "HOME"
	case model.ViewLibrary:
		return "LIBRARY"
	case model.ViewSearch:
		return "SEARCH"
	case model.ViewPlaylists:
		return "PLAYLISTS"
	case model.ViewVisualizer:
		return "VISUALIZER"
	case model.ViewLyrics:
		return "LYRICS"
	case model.ViewSettings:
		return "SETTINGS"
	default:
		return "musicTUI"
	}
}

func (a App) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	th := a.theme
	bc := th.Border // all inner panels use this border color

	// ── SONICA-style 6-panel grid layout ──
	// Title line at top, 3-column grid in the middle, status bar at bottom.
	// No outer frame border — the panels' own borders create the structure.

	// Column widths — side panels ~25% each, center ~50%
	leftW := a.width * 25 / 100
	rightW := a.width * 25 / 100
	if leftW < 18 {
		leftW = 18
	}
	if rightW < 20 {
		rightW = 20
	}
	centerW := a.width - leftW - rightW
	if centerW < 20 {
		centerW = 20
	}

	// Heights: title line (1) + grid (fill) + status bar (1)
	gridH := a.height - 2
	if gridH < 10 {
		gridH = 10
	}

	// ══════════════════════════════════════════════════════════════
	// LEFT COLUMN: NAVIGATION + PLAYLISTS (single bordered column)
	// ══════════════════════════════════════════════════════════════
	// Size to content: nav items + 1 buffer, playlists gets the rest
	navLines := len(a.sidebar.Items) + 1
	plLines := gridH - navLines - 3 // -3 for top border + divider + bottom border
	if plLines < 3 {
		plLines = 3
	}

	navContent := a.sidebar.ViewContent(th, leftW-2, navLines)

	var plContent string
	if len(a.playlist.Items) > 0 {
		// Scrollable playlist list with selection highlight
		startIdx := 0
		if a.playlist.Selected >= plLines {
			startIdx = a.playlist.Selected - plLines + 1
		}
		for i := startIdx; i < len(a.playlist.Items) && i < startIdx+plLines; i++ {
			pl := a.playlist.Items[i]
			name := pl.Name
			if len(name) > leftW-6 {
				name = name[:leftW-7] + "…"
			}
			if i > startIdx {
				plContent += "\n"
			}
			if i == a.playlist.Selected && a.view == model.ViewPlaylists {
				plContent += lipgloss.NewStyle().Foreground(th.Accent).Bold(true).
					Render(fmt.Sprintf(" ▸%s", name))
			} else {
				plContent += lipgloss.NewStyle().Foreground(th.FgDim).
					Render(fmt.Sprintf("  %s", name))
			}
		}
	} else if a.playlist.Loading {
		plContent = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(" Loading...")
	} else {
		plContent = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(" No playlists")
	}

	leftBc := bc
	if a.focus == model.FocusSidebar {
		leftBc = th.BorderFocused
	}
	leftCol := components.MultiSectionColumn(
		[]components.PanelSection{
			{Title: "NAVIGATION", Content: navContent, Lines: navLines},
			{Title: "PLAYLISTS", Content: plContent, Lines: plLines},
		},
		leftW, gridH, leftBc, th.Surface, th,
	)

	// ══════════════════════════════════════════════════════════════
	// CENTER COLUMN: NOW PLAYING / view content (full height)
	// ══════════════════════════════════════════════════════════════
	centerBc := bc
	if a.focus == model.FocusContent {
		centerBc = th.BorderFocused
	}
	rightBc := bc
	if a.focus == model.FocusRight {
		rightBc = th.BorderFocused
	}

	centerInnerW := centerW - 2
	centerInnerH := gridH - 2

	// Build center panel content: now-playing info at top + current view below
	var centerContent string

	// Now-playing header (if playing)
	if a.playback.Track != nil {
		npContent := a.playing.PanelContent(th, a.playback, centerInnerW)
		separator := lipgloss.NewStyle().Foreground(th.Border).Render(
			"\n" + strings.Repeat("─", centerInnerW) + "\n")
		centerContent = npContent + separator
	}

	// View content below now-playing
	viewH := centerInnerH
	if a.playback.Track != nil {
		viewH -= 6 // now-playing header takes ~5 lines + separator
	}
	if viewH < 4 {
		viewH = 4
	}

	var viewContent string
	switch a.view {
	case model.ViewHome:
		viewContent = a.home.View(th, centerInnerW, viewH)
	case model.ViewLibrary:
		viewContent = a.library.View(th, centerInnerW, viewH)
	case model.ViewSearch:
		viewContent = a.search.View(th, centerInnerW, viewH)
	case model.ViewPlaylists:
		viewContent = a.playlist.View(th, centerInnerW, viewH)
	case model.ViewLyrics:
		viewContent = a.lyrics.View(th, centerInnerW, viewH, a.playback.Position.Milliseconds())
	default:
		viewContent = lipgloss.NewStyle().
			Foreground(th.FgMuted).Italic(true).
			Render("  Coming soon...")
	}

	// Status message
	if a.status != "" {
		statusIcon := lipgloss.NewStyle().Foreground(th.Accent).Render("● ")
		statusText := lipgloss.NewStyle().Foreground(th.FgDim).Render(a.status)
		viewContent += "\n" + statusIcon + statusText
	}

	// Inline lyrics below content (toggle with 'l' key)
	if a.showLyrics && a.playback.Track != nil && a.view != model.ViewLyrics {
		lyricsH := viewH / 2 // use bottom half of remaining space
		if lyricsH < 4 {
			lyricsH = 4
		}
		lyricsSep := lipgloss.NewStyle().Foreground(th.Border).Render(
			"\n" + strings.Repeat("─", centerInnerW) + "\n")
		lyricsHeader := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(" LYRICS") +
			lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render("  (press l to hide)") + "\n"
		lyricsContent := a.lyrics.View(th, centerInnerW, lyricsH, a.playback.Position.Milliseconds())
		viewContent += lyricsSep + lyricsHeader + lyricsContent
	}

	centerTitle := "NOW PLAYING"
	if a.playback.Track == nil {
		centerTitle = a.viewTitle()
	}
	centerContent += viewContent
	centerPanel := components.TitledPanel(centerTitle, centerContent, centerW, gridH, centerBc, th)

	// ══════════════════════════════════════════════════════════════
	// RIGHT COLUMN: TRACKLIST + ARTWORK + VISUALIZER (single bordered column)
	// ══════════════════════════════════════════════════════════════
	// Size right column: artwork and visualizer are compact, tracklist gets the rest
	// Split right column: tracklist 50%, artwork 30%, visualizer 20%
	artLines := (gridH - 4) * 30 / 100
	vizLines := (gridH - 4) * 20 / 100
	tlLines := gridH - 4 - artLines - vizLines
	if tlLines < 3 {
		tlLines = 3
	}
	if artLines < 5 {
		artLines = 5
	}
	if vizLines < 2 {
		vizLines = 2
	}

	// Tracklist content — shows the play queue
	var tlContent string
	if !a.queue.IsEmpty() {
		// Scroll window: show tracks around the current index
		startIdx := 0
		if a.queue.Index > tlLines/2 {
			startIdx = a.queue.Index - tlLines/2
		}
		for i := startIdx; i < a.queue.Len() && i < startIdx+tlLines; i++ {
			t := a.queue.Tracks[i]
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(th.FgDim)
			if i == a.queue.Index {
				prefix = ">*"
				style = lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
			}
			name := t.Name
			if len(name) > rightW-8 {
				name = name[:rightW-9] + "…"
			}
			line := style.Render(fmt.Sprintf("%s%02d %s", prefix, i+1, name))
			if i > startIdx {
				tlContent += "\n"
			}
			tlContent += line
		}
	} else {
		tlContent = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).Render(" No active queue")
	}

	// Artwork content
	artContent := a.artwork.View(th, rightW-2, artLines)

	// Visualizer: animated bars
	vizContent := a.viz.View(th, rightW-2, vizLines)

	rightCol := components.MultiSectionColumn(
		[]components.PanelSection{
			{Title: "TRACKLIST", Content: tlContent, Lines: tlLines},
			{Title: "ARTWORK", Content: artContent, Lines: artLines},
			{Title: "VISUALIZER", Content: vizContent, Lines: vizLines},
		},
		rightW, gridH, rightBc, th.Surface, th,
	)

	// ══════════════════════════════════════════════════════════════
	// ASSEMBLE: title + 3-column grid + status bar
	// ══════════════════════════════════════════════════════════════
	grid := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, centerPanel, rightCol)

	// Title line: ─── musicTUI | Terminal Music Player | ● username ───
	titleStr := "musicTUI | Terminal Music Player"
	if a.home.Username != "" {
		titleStr += " | ● " + a.home.Username
	}
	titleText := lipgloss.NewStyle().Foreground(th.Accent).Bold(true).Render(titleStr)
	titleVisualW := lipgloss.Width(titleText)
	titleDashL := (a.width - titleVisualW - 2) / 2
	titleDashR := a.width - titleDashL - titleVisualW - 2
	if titleDashL < 1 {
		titleDashL = 1
	}
	if titleDashR < 1 {
		titleDashR = 1
	}
	borderStyle := lipgloss.NewStyle().Foreground(th.Border)
	titleLine := borderStyle.Render(strings.Repeat("─", titleDashL)) + " " +
		titleText + " " +
		borderStyle.Render(strings.Repeat("─", titleDashR))

	// Status bar (full width)
	statusBar := a.playing.StatusBarView(th, a.playback, a.width)

	return titleLine + "\n" + grid + "\n" + statusBar
}
