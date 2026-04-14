package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	spotifylib "github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"

	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/mpris"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/theme"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
	"github.com/iamteedoh/musicTUI/internal/update"
)

type App struct {
	config   config.Config
	theme    theme.Theme
	width    int
	height   int
	focus              model.FocusMode
	view               model.View
	showLyrics         bool // toggle inline lyrics in center panel (default: true)
	sidebarPlaylistFocus bool // true when sidebar focus is on the playlist section
	modal    components.Modal
	onboard  components.Onboard
	help     components.Help
	prevView model.View // view to return to when ? help is closed
	sidebar  components.Sidebar
	home     components.Home
	library  components.Library
	search   components.Search
	playlist components.Playlists
	playing  components.NowPlaying
	viz      components.MiniVisualizer
	lyrics   components.Lyrics
	artwork  components.Artwork
	settings components.Settings
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

	// MPRIS media keys
	mprisServer *mpris.Server

	// Versioning / self-update
	version         string
	latestVersion   string
	latestRelease   *update.Release
	updateAvailable bool
	updating        bool

	// Stale-session recovery: when librespot reports a track as Unavailable
	// (typically post-sleep-wake or after long idle), we hold the failed
	// track here while we refresh the OAuth token. Once AuthSuccessMsg
	// lands with a fresh token (and the engine has been re-seeded), we
	// re-dispatch PlayTrack for this track once.
	pendingRetryTrack *model.Track
}

func NewApp(cfg config.Config, bridgePath string, version string) App {
	th := theme.FromName(cfg.Theme)
	app := App{
		config:   cfg,
		theme:    th,
		version:  version,
		focus:    model.FocusSidebar,
		view:     model.ViewHome,
		sidebar:  components.NewSidebar(),
		home:     components.NewHome(),
		onboard:  components.NewOnboard(),
		help:     components.NewHelp(),
		library:  components.NewLibrary(),
		search:   components.NewSearch(),
		playlist: components.NewPlaylists(),
		playing:  components.NewNowPlaying(),
		viz:      components.NewMiniVisualizer(),
		lyrics:   components.NewLyrics(),
		artwork:  components.NewArtwork(),
		settings: components.NewSettings(),
		showLyrics: true,
		playback:   model.PlaybackState{Volume: cfg.Volume},
		queue:      model.NewQueue(),
		bridgePath: bridgePath,
	}
	if cfg.Spotify.ClientID != "" {
		app.auth = sp.NewAuth(cfg.Spotify.ClientID)
	} else {
		// First launch with no credentials — walk the user through setup.
		app.onboard.Start()
	}
	app.mprisServer = mpris.New() // nil if D-Bus unavailable
	return app
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd()}

	// Try cached auth on startup
	if a.config.Spotify.ClientID != "" {
		cmds = append(cmds, a.tryCachedAuthCmd())
	}

	// Start MPRIS media key listener
	if a.mprisServer != nil {
		cmds = append(cmds, listenMprisCmd(a.mprisServer))
	}

	// Async check for a newer release; result arrives as UpdateCheckResultMsg.
	cmds = append(cmds, CheckForUpdateCmd(a.version))

	return tea.Batch(cmds...)
}

type mprisCommandMsg struct{ Cmd mpris.MediaCommand }

func listenMprisCmd(srv *mpris.Server) tea.Cmd {
	return func() tea.Msg {
		cmd, ok := <-srv.Commands()
		if !ok {
			return nil
		}
		return mprisCommandMsg{Cmd: cmd}
	}
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
		client := sp.NewClient(spotifylib.New(httpClient), httpClient)
		username, err := client.FetchUsername(context.Background())
		if err != nil {
			// Token is stale/revoked — clear it and prompt for fresh login
			sp.ClearToken()
			return StatusMsg("Session expired — press Ctrl+L to re-authenticate")
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

func (a App) startInteractiveAuthCmd() tea.Cmd {
	return func() tea.Msg {
		// Clear stale credentials so the next startup doesn't try a revoked token
		sp.ClearToken()

		url := a.auth.AuthURL()
		openBrowser(url)

		return AuthURLMsg{URL: url}
	}
}

// openBrowser attempts to open the given URL in the user's default browser.
// Failures are silent — the URL is also surfaced in the UI as a fallback.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (a App) waitForAuthCallbackCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		tok, err := a.auth.WaitForCallback(ctx)
		if err != nil {
			return AuthErrorMsg{Err: fmt.Errorf("login failed: %w", err)}
		}

		httpClient := a.auth.HTTPClient(tok)
		client := sp.NewClient(spotifylib.New(httpClient), httpClient)
		username, err := client.FetchUsername(context.Background())
		if err != nil {
			return AuthErrorMsg{Err: fmt.Errorf("failed to fetch user: %w", err)}
		}

		return AuthSuccessMsg{Client: client, Username: username, AccessToken: tok.AccessToken}
	}
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return a.handleKey(msg)
	case mprisCommandMsg:
		var cmds []tea.Cmd
		switch msg.Cmd {
		case mpris.CmdPlayPause:
			if a.engine != nil {
				if a.playback.IsPlaying {
					_ = a.engine.Pause()
				} else {
					_ = a.engine.Resume()
				}
			}
		case mpris.CmdNext:
			if next := a.queue.Next(); next != nil {
				cmds = append(cmds, func() tea.Msg { return PlayTrackMsg{Track: *next} })
			}
		case mpris.CmdPrevious:
			if prev := a.queue.Previous(); prev != nil {
				cmds = append(cmds, func() tea.Msg { return PlayTrackMsg{Track: *prev} })
			}
		case mpris.CmdStop:
			if a.engine != nil {
				_ = a.engine.Stop()
			}
		}
		// Re-listen for next MPRIS command
		if a.mprisServer != nil {
			cmds = append(cmds, listenMprisCmd(a.mprisServer))
		}
		if len(cmds) > 0 {
			return a, tea.Batch(cmds...)
		}
		return a, nil
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		return a, nil
	case TickMsg:
		a.viz.SetPosition(a.playback.Position.Milliseconds())
		a.viz.Update(a.playback.IsPlaying)
		return a, tickCmd()

	// Auth
	case AuthURLMsg:
		a.status = "Login URL opened — complete auth in browser"
		a.home.AuthURL = msg.URL
		return a, a.waitForAuthCallbackCmd()
	case AuthSuccessMsg:
		a.client = msg.Client
		a.home.Username = msg.Username
		a.home.AuthURL = ""
		a.accessToken = msg.AccessToken
		a.status = "" // Auth info shown in home view and title bar

		// Create audio engine (lazy — bridge subprocess starts on first play).
		// On re-auth, keep the existing engine but push the refreshed token
		// so the next play uses tokens with the current scope set.
		if a.bridgePath != "" {
			if a.engine == nil {
				a.engine = audio.NewEngine(a.bridgePath, msg.AccessToken)
				a.viz.SetSpectrum(a.engine.Spectrum)
			} else {
				a.engine.SetToken(msg.AccessToken)
			}
		}

		// If a track failed with Unavailable and we kicked off a token
		// refresh, retry the track now that the engine has the fresh
		// token. Fire-and-forget: if this retry also fails, the normal
		// error path takes over (pendingRetryTrack stays nil this time).
		if a.pendingRetryTrack != nil {
			retry := *a.pendingRetryTrack
			a.pendingRetryTrack = nil
			a.status = "Retrying playback..."
			return a, tea.Batch(
				FetchPlaylistsCmd(msg.Client, 0),
				func() tea.Msg { return PlayTrackMsg{Track: retry} },
			)
		}
		// Auto-load playlists for the left panel
		return a, FetchPlaylistsCmd(msg.Client, 0)
	case AuthErrorMsg:
		// Stale-session retry failed — clear the pending track so we
		// don't loop, and surface a clean status message.
		a.pendingRetryTrack = nil
		a.status = "Auth error: " + msg.Err.Error()
		return a, nil

	// Self-update
	case UpdateCheckResultMsg:
		if msg.Release != nil {
			a.updateAvailable = true
			a.latestVersion = msg.Release.TagName
			a.latestRelease = msg.Release
			a.home.UpdateAvailable = msg.Release.TagName
		}
		return a, nil
	case UpdateStartedMsg:
		a.updating = true
		a.status = "Downloading update..."
		return a, nil
	case UpdateAppliedMsg:
		a.updating = false
		a.status = fmt.Sprintf("Updated to %s — please restart musicTUI to finish.", msg.NewVersion)
		return a, nil
	case UpdateFailedMsg:
		a.updating = false
		a.status = "Update failed: " + msg.Err.Error()
		return a, nil

	// Data loaded
	case LibraryLoadedMsg:
		a.library.AppendTracks(msg.Tracks, msg.Total, msg.Offset)
		return a, nil
	case PlaylistsLoadedMsg:
		a.playlist.Items = append(a.playlist.Items, msg.Playlists...)
		a.playlist.Total = msg.Total
		// Auto-paginate: fetch next page if more playlists exist
		if uint32(len(a.playlist.Items)) < msg.Total {
			return a, FetchPlaylistsCmd(a.client, len(a.playlist.Items))
		}
		// Sort alphabetically once all pages are loaded
		sort.Slice(a.playlist.Items, func(i, j int) bool {
			return strings.ToLower(a.playlist.Items[i].Name) < strings.ToLower(a.playlist.Items[j].Name)
		})
		a.playlist.Loading = false

		// Opt-in cleanup prompts. Disabled by default because these
		// actions unfollow playlists (Spotify's API has no true delete),
		// and unfollowing a playlist you own removes it from /me/playlists
		// with no public API to list it back. Explicit warning in the
		// prompt copy.
		if a.config.CheckDuplicates {
			if groups := a.findDuplicatePlaylists(); len(groups) > 0 {
				var names []string
				for _, g := range groups {
					names = append(names, fmt.Sprintf("\"%s\" (%d copies)", g[0].Name, len(g)))
				}
				dupMsg := "Duplicate playlists found:\n\n"
				for _, n := range names {
					dupMsg += "  " + n + "\n"
				}
				dupMsg += "\nMerge into one playlist each?\n" +
					"Extras will be unfollowed (not deleted). If you own them\n" +
					"you will not be able to list them again via this app."
				a.modal.ShowConfirm("Merge Duplicate Playlists", dupMsg, components.ActionConsolidateDuplicates, "")
				return a, nil
			}
			if empty := a.findEmptyPlaylists(); len(empty) > 0 {
				emptyMsg := fmt.Sprintf("%d empty playlist(s) found:\n\n", len(empty))
				for _, pl := range empty {
					emptyMsg += fmt.Sprintf("  \"%s\"\n", pl.Name)
				}
				emptyMsg += "\nRemove them from your library?\n" +
					"These will be unfollowed (not deleted). If you own them\n" +
					"you will not be able to list them again via this app."
				a.modal.ShowConfirm("Remove Empty Playlists", emptyMsg, components.ActionDeleteEmptyPlaylists, "")
			}
		}
		return a, nil
	case PlaylistTracksLoadedMsg:
		a.playlist.Tracks = append(a.playlist.Tracks, msg.Tracks...)
		a.playlist.TracksTotal = msg.Total
		// Auto-paginate: fetch next page if more tracks exist
		if uint32(len(a.playlist.Tracks)) < msg.Total {
			return a, FetchPlaylistTracksCmd(a.client, msg.PlaylistID, len(a.playlist.Tracks))
		}
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
	// Playlist mutations
	case PlaylistCreatedMsg:
		a.playlist.Items = append(a.playlist.Items, msg.Playlist)
		a.playlist.Total++
		sort.Slice(a.playlist.Items, func(i, j int) bool {
			return strings.ToLower(a.playlist.Items[i].Name) < strings.ToLower(a.playlist.Items[j].Name)
		})
		a.status = "Created playlist: " + msg.Playlist.Name
		return a, nil
	case PlaylistUpdatedMsg:
		for i, pl := range a.playlist.Items {
			if pl.ID == msg.PlaylistID {
				a.playlist.Items[i].Name = msg.NewName
				a.playlist.Items[i].Description = msg.NewDesc
				break
			}
		}
		sort.Slice(a.playlist.Items, func(i, j int) bool {
			return strings.ToLower(a.playlist.Items[i].Name) < strings.ToLower(a.playlist.Items[j].Name)
		})
		if a.playlist.CurrentID == msg.PlaylistID {
			a.playlist.CurrentName = msg.NewName
		}
		a.status = "Updated playlist: " + msg.NewName
		return a, nil
	case PlaylistDeletedMsg:
		for i, pl := range a.playlist.Items {
			if pl.ID == msg.PlaylistID {
				a.playlist.Items = append(a.playlist.Items[:i], a.playlist.Items[i+1:]...)
				a.playlist.Total--
				if a.playlist.Selected >= len(a.playlist.Items) && a.playlist.Selected > 0 {
					a.playlist.Selected--
				}
				break
			}
		}
		if a.playlist.CurrentID == msg.PlaylistID {
			a.playlist.Back()
		}
		a.status = "Playlist removed"
		return a, nil
	case TrackAddedToPlaylistMsg:
		for i, pl := range a.playlist.Items {
			if pl.ID == msg.PlaylistID {
				a.playlist.Items[i].TrackCount++
				break
			}
		}
		a.status = "Track added to playlist"
		return a, nil
	case TrackRemovedFromPlaylistMsg:
		for i, t := range a.playlist.Tracks {
			if t.URI == msg.TrackURI {
				a.playlist.Tracks = append(a.playlist.Tracks[:i], a.playlist.Tracks[i+1:]...)
				if a.playlist.TracksTotal > 0 {
					a.playlist.TracksTotal--
				}
				if a.playlist.TrackSelected >= len(a.playlist.Tracks) && a.playlist.TrackSelected > 0 {
					a.playlist.TrackSelected--
				}
				break
			}
		}
		a.status = "Track removed from playlist"
		return a, nil
	case TrackMovedMsg:
		// Remove from source playlist's local tracks
		for i, t := range a.playlist.Tracks {
			if t.URI == msg.TrackURI {
				a.playlist.Tracks = append(a.playlist.Tracks[:i], a.playlist.Tracks[i+1:]...)
				if a.playlist.TracksTotal > 0 {
					a.playlist.TracksTotal--
				}
				if a.playlist.TrackSelected >= len(a.playlist.Tracks) && a.playlist.TrackSelected > 0 {
					a.playlist.TrackSelected--
				}
				break
			}
		}
		// Update track counts
		for i, pl := range a.playlist.Items {
			if pl.ID == msg.FromPlaylistID && a.playlist.Items[i].TrackCount > 0 {
				a.playlist.Items[i].TrackCount--
			}
			if pl.ID == msg.ToPlaylistID {
				a.playlist.Items[i].TrackCount++
			}
		}
		a.status = "Track moved to playlist"
		return a, nil
	case DuplicatesConsolidatedMsg:
		a.playlist.Items = nil
		a.playlist.Total = 0
		a.playlist.Loading = true
		a.status = fmt.Sprintf("Consolidated %d duplicate group(s), removed %d playlist(s)", msg.MergedCount, msg.DeletedCount)
		if a.client != nil {
			return a, FetchPlaylistsCmd(a.client, 0)
		}
		return a, nil
	case EmptyPlaylistsDeletedMsg:
		a.playlist.Items = nil
		a.playlist.Total = 0
		a.playlist.Loading = true
		a.status = fmt.Sprintf("Deleted %d empty playlist(s)", msg.DeletedCount)
		if a.client != nil {
			return a, FetchPlaylistsCmd(a.client, 0)
		}
		return a, nil

	case DataErrorMsg:
		a.status = "Error: " + msg.Err.Error()
		a.library.Loading = false
		a.playlist.Loading = false
		if a.playlist.TracksLoading {
			a.playlist.TracksLoading = false
			a.playlist.Error = msg.Err.Error()
		}
		a.search.Loading = false
		return a, nil
	case StatusMsg:
		a.status = string(msg)
		// If a stale-session retry was pending but auth came back as a
		// plain status (no AuthSuccessMsg), drop the retry so we don't
		// hang waiting for a success that'll never arrive.
		a.pendingRetryTrack = nil
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
			// If the bridge exited before we ever reached "playing" it likely
			// crashed (librespot auth error, audio device failure, etc.).
			// Point the user at the log so they can see what went wrong.
			if strings.HasPrefix(a.status, "Loading track") {
				a.status = "Playback stopped before starting — see " + audio.LogPath()
			}
		case "error":
			// "Bad credentials" from librespot means the cached OAuth token
			// no longer satisfies the streaming service (commonly because the
			// scopes it was issued with are now insufficient). Purge the
			// token so the user can re-authenticate with a fresh set.
			errMsg := msg.Event.Error
			if strings.Contains(errMsg, "Bad credentials") {
				sp.ClearToken()
				a.client = nil
				a.home.Username = ""
				a.status = "Session needs refreshing — press Ctrl+L to re-authenticate"
				return a, nil
			}
			// "Unavailable: spotify:track:..." typically means the librespot
			// session is stale (post-sleep, long idle). We transparently
			// refresh the OAuth token and retry the track once so the user
			// doesn't have to quit and relaunch.
			if strings.HasPrefix(errMsg, "Unavailable") && a.pendingRetryTrack == nil && a.playback.Track != nil && a.config.Spotify.ClientID != "" {
				retry := *a.playback.Track
				a.pendingRetryTrack = &retry
				a.status = "Session expired — refreshing and retrying..."
				return a, a.tryCachedAuthCmd()
			}
			a.status = "Playback error: " + errMsg + "  (see " + audio.LogPath() + ")"
		case "loading":
			a.status = "Loading track..."
		}
		// Re-register the event listener only if the bridge is still alive.
		// After bridge death, readEvents() resets Started() to false —
		// a new listener will be registered when PlayTrack restarts the bridge.
		if a.engine != nil && a.engine.Started() {
			return a, ListenForAudioEvents(a.engine)
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Onboarding wizard takes over input while active
	if a.onboard.Active {
		return a.handleOnboardKey(msg)
	}
	// Modal captures all input when active
	if a.modal.Active {
		return a.handleModalKey(msg)
	}

	// Global keys
	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit
	case "ctrl+u":
		if a.updateAvailable && !a.updating && a.latestRelease != nil {
			a.updating = true
			a.status = "Downloading " + a.latestVersion + "..."
			return a, ApplyUpdateCmd(a.latestRelease)
		}
		return a, nil
	case "ctrl+l":
		if a.auth == nil {
			// Re-open the onboarding wizard for users who escaped it.
			a.onboard.Start()
			return a, nil
		}
		a.status = "Opening browser for Spotify login..."
		return a, a.startInteractiveAuthCmd()
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
	case "?":
		// Don't swallow `?` while the user is typing in the search input.
		if a.isSearchInputFocused() {
			break
		}
		if a.view != model.ViewHelp {
			a.prevView = a.view
			a.view = model.ViewHelp
			a.help.Reset()
		}
		return a, nil
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

func (a App) handleOnboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := msg.String()

	switch s {
	case "ctrl+c":
		return a, tea.Quit
	case "esc":
		a.onboard.Close()
		return a, nil
	case "left", "h":
		a.onboard.Prev()
		return a, nil
	}

	if a.onboard.OnFinalStep() {
		switch s {
		case "enter":
			clientID := a.onboard.ClientID()
			if clientID == "" {
				a.onboard.Error = "Client ID can't be empty"
				return a, nil
			}
			a.config.Spotify.ClientID = clientID
			if err := config.Save(a.config); err != nil {
				a.onboard.Error = "Failed to save config: " + err.Error()
				return a, nil
			}
			a.auth = sp.NewAuth(clientID)
			a.onboard.Close()
			a.status = "Opening browser for Spotify login..."
			return a, a.startInteractiveAuthCmd()
		case "backspace":
			a.onboard.Backspace()
			return a, nil
		default:
			if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				a.onboard.InputChar(rune(s[0]))
				return a, nil
			}
			for _, r := range msg.Runes {
				a.onboard.InputChar(r)
			}
			return a, nil
		}
	}

	// Non-final steps: advance on enter / right, open browser on 'o'.
	switch s {
	case "enter", "right", "l":
		a.onboard.Next()
		return a, nil
	case "o", "O":
		if a.onboard.Step == 1 {
			openBrowser("https://developer.spotify.com/dashboard")
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch a.modal.Kind {
	case components.ModalConfirm:
		switch msg.String() {
		case "y", "Y":
			switch a.modal.Action {
			case components.ActionDeletePlaylist:
				id := a.modal.TargetID
				a.modal.Close()
				if a.client != nil {
					return a, DeletePlaylistCmd(a.client, id)
				}
			case components.ActionRemoveTrack:
				id := a.modal.TargetID
				uri := a.modal.TrackURI
				a.modal.Close()
				if a.client != nil {
					return a, RemoveTrackFromPlaylistCmd(a.client, id, uri)
				}
			case components.ActionConsolidateDuplicates:
				groups := a.findDuplicatePlaylists()
				a.modal.Close()
				a.status = "Consolidating duplicate playlists..."
				if a.client != nil && len(groups) > 0 {
					return a, ConsolidateDuplicatesCmd(a.client, groups)
				}
			case components.ActionDeleteEmptyPlaylists:
				empty := a.findEmptyPlaylists()
				a.modal.Close()
				a.status = "Deleting empty playlists..."
				if a.client != nil && len(empty) > 0 {
					return a, DeleteEmptyPlaylistsCmd(a.client, empty)
				}
			}
			a.modal.Close()
		case "n", "N", "esc":
			a.modal.Close()
		}

	case components.ModalInput:
		s := msg.String()
		switch s {
		case "enter":
			name := strings.TrimSpace(a.modal.Input1)
			if name != "" && a.client != nil {
				desc := a.modal.Input2
				action := a.modal.Action
				targetID := a.modal.TargetID
				a.modal.Close()
				switch action {
				case components.ActionCreatePlaylist:
					return a, CreatePlaylistCmd(a.client, name, desc)
				case components.ActionEditPlaylist:
					return a, UpdatePlaylistCmd(a.client, targetID, name, desc)
				}
			}
		case "esc":
			a.modal.Close()
		case "tab":
			a.modal.TabField()
		case "backspace":
			a.modal.Backspace()
		default:
			if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				a.modal.InputChar(rune(s[0]))
			} else if len(msg.Runes) > 0 {
				for _, r := range msg.Runes {
					a.modal.InputChar(r)
				}
			}
		}

	case components.ModalPicker:
		switch msg.String() {
		case "j", "down":
			a.modal.Down()
		case "k", "up":
			a.modal.Up()
		case "enter":
			if a.modal.PickerSelected < len(a.modal.PickerItems) && a.client != nil {
				pl := a.modal.PickerItems[a.modal.PickerSelected]
				uri := a.modal.TrackURI
				action := a.modal.Action
				fromID := a.modal.TargetID
				a.modal.Close()
				if action == components.ActionMoveTrack {
					return a, MoveTrackCmd(a.client, fromID, pl.ID, uri)
				}
				return a, AddTrackToPlaylistCmd(a.client, pl.ID, uri)
			}
		case "esc":
			a.modal.Close()
		}
	}
	return a, nil
}

// findDuplicatePlaylists returns groups of playlists that share the same name.
func (a App) findDuplicatePlaylists() [][]model.Playlist {
	byName := make(map[string][]model.Playlist)
	for _, pl := range a.playlist.Items {
		key := strings.ToLower(strings.TrimSpace(pl.Name))
		byName[key] = append(byName[key], pl)
	}
	var groups [][]model.Playlist
	for _, group := range byName {
		if len(group) >= 2 {
			groups = append(groups, group)
		}
	}
	return groups
}

func (a App) findEmptyPlaylists() []model.Playlist {
	var empty []model.Playlist
	for _, pl := range a.playlist.Items {
		if pl.TrackCount == 0 {
			empty = append(empty, pl)
		}
	}
	return empty
}

func (a App) isSearchInputFocused() bool {
	return a.view == model.ViewSearch && !a.search.ResultsFocused
}

func (a App) handleNavKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if a.sidebarPlaylistFocus {
		return a.handleSidebarPlaylistKey(msg)
	}
	switch msg.String() {
	case "j", "down":
		if a.sidebar.Selected >= len(a.sidebar.Items)-1 && len(a.playlist.Items) > 0 {
			// Past last nav item → enter playlist section
			a.sidebarPlaylistFocus = true
			a.view = model.ViewPlaylists
			a.playlist.Mode = components.PlaylistModeList
			return a, nil
		}
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
		if a.view == model.ViewSearch {
			a.search.ResultsFocused = false
		}
		return a, a.onViewEnter()
	case "d", "x", "c", "e":
		// Forward playlist management keys when on the Playlists nav item
		if a.view == model.ViewPlaylists && len(a.playlist.Items) > 0 {
			a.sidebarPlaylistFocus = true
			return a.handleSidebarPlaylistKey(msg)
		}
	}
	return a, nil
}

func (a App) handleSidebarPlaylistKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if a.playlist.Selected < len(a.playlist.Items)-1 {
			a.playlist.Selected++
		}
	case "k", "up":
		if a.playlist.Selected <= 0 {
			// Back to navigation section
			a.sidebarPlaylistFocus = false
			return a, nil
		}
		a.playlist.Selected--
	case "enter", "l":
		if a.playlist.Selected < len(a.playlist.Items) {
			a.view = model.ViewPlaylists
			a.focus = model.FocusContent
			a.sidebarPlaylistFocus = false
			// Open the selected playlist's tracks
			pl := a.playlist.Items[a.playlist.Selected]
			a.playlist.Mode = components.PlaylistModeTracks
			a.playlist.CurrentID = pl.ID
			a.playlist.CurrentName = pl.Name
			a.playlist.Tracks = nil
			a.playlist.TrackSelected = 0
			a.playlist.TracksLoading = true
			a.playlist.Error = ""
			if a.client != nil {
				return a, FetchPlaylistTracksCmd(a.client, pl.ID, 0)
			}
		}
	case "d", "x":
		if a.playlist.Selected < len(a.playlist.Items) {
			pl := a.playlist.Items[a.playlist.Selected]
			// Always confirm — an empty playlist that's yours can still
			// only be "unfollowed" via the Spotify API, and the API has
			// no way to list it back afterwards. No instant-delete.
			msg := fmt.Sprintf(
				"Remove \"%s\" (%d tracks) from your library?\n\n"+
					"This unfollows the playlist. You'll still own it on Spotify, but it will\n"+
					"disappear from musicTUI and cannot be restored automatically.",
				pl.Name, pl.TrackCount,
			)
			a.modal.ShowConfirm("Remove from Library", msg,
				components.ActionDeletePlaylist, pl.ID)
		}
	case "c":
		a.modal.ShowInput("Create Playlist", "Name", "", "Description (optional)", "", components.ActionCreatePlaylist, "")
	case "e":
		if a.playlist.Selected < len(a.playlist.Items) {
			pl := a.playlist.Items[a.playlist.Selected]
			a.modal.ShowInput("Edit Playlist", "Name", pl.Name, "Description", pl.Description, components.ActionEditPlaylist, pl.ID)
		}
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
	case model.ViewHelp:
		switch msg.String() {
		case "j", "down":
			a.help.ScrollDown()
		case "k", "up":
			a.help.ScrollUp()
		case "esc", "h", "?":
			// Return to whatever view we came from; fall back to Home.
			target := a.prevView
			if target == model.ViewHelp {
				target = model.ViewHome
			}
			a.view = target
			for i, item := range a.sidebar.Items {
				if item.View == target {
					a.sidebar.Selected = i
					break
				}
			}
		}
	case model.ViewSettings:
		switch msg.String() {
		case "j", "down":
			a.settings.Down()
		case "k", "up":
			a.settings.Up()
		case "enter":
			switch a.settings.SelectedKey() {
			case "check_duplicates":
				a.config.CheckDuplicates = !a.config.CheckDuplicates
				_ = config.Save(a.config)
			}
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
	case "a":
		if t := a.library.SelectedTrack(); t != nil && len(a.playlist.Items) > 0 {
			a.modal.ShowPicker("Add to Playlist", a.playlist.Items, t.URI)
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
		case "a":
			kind, item := a.search.SelectedItem()
			if kind == "track" {
				if t, ok := item.(model.Track); ok && len(a.playlist.Items) > 0 {
					a.modal.ShowPicker("Add to Playlist", a.playlist.Items, t.URI)
				}
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
		if a.playlist.Mode == components.PlaylistModeTracks {
			// Play selected track, queue all playlist tracks
			if t := a.playlist.SelectedTrack(); t != nil {
				tracks := a.playlist.Tracks
				idx := a.playlist.TrackSelected
				return a, func() tea.Msg { return PlayQueueMsg{Tracks: tracks, StartIdx: idx} }
			}
		} else if id, ok := a.playlist.Select(); ok && a.client != nil {
			return a, FetchPlaylistTracksCmd(a.client, id, 0)
		}
	case "d", "x":
		if a.playlist.Mode == components.PlaylistModeTracks {
			if t := a.playlist.SelectedTrack(); t != nil {
				a.modal.ShowConfirm("Remove Track",
					fmt.Sprintf("Remove \"%s\" from this playlist? (y/n)", t.Name),
					components.ActionRemoveTrack, a.playlist.CurrentID)
				a.modal.TrackURI = t.URI
			}
		}
	case "m":
		if a.playlist.Mode == components.PlaylistModeTracks {
			if t := a.playlist.SelectedTrack(); t != nil && len(a.playlist.Items) > 0 {
				a.modal.ShowPicker("Move to Playlist", a.playlist.Items, t.URI)
				a.modal.Action = components.ActionMoveTrack
				a.modal.TargetID = a.playlist.CurrentID // source playlist
			}
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

	if a.onboard.Active {
		return a.onboard.View(a.theme, a.width, a.height)
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
			if i == a.playlist.Selected && (a.sidebarPlaylistFocus || a.view == model.ViewPlaylists) {
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
		// If the user is authenticated but no playlists came back, point
		// them at the Playlists view where the full User-Management fix
		// is explained. "No playlists found" alone is misleading when
		// they actually have playlists on Spotify.
		if a.client != nil {
			plContent = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
				Render(" None — see Playlists view")
		} else {
			plContent = lipgloss.NewStyle().Foreground(th.FgMuted).Italic(true).
				Render(" No playlists")
		}
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

	// Reserve space for inline lyrics when shown
	inlineLyrics := a.showLyrics && a.playback.Track != nil && a.view != model.ViewLyrics
	lyricsH := 0
	if inlineLyrics {
		lyricsH = viewH / 3
		if lyricsH < 4 {
			lyricsH = 4
		}
		viewH -= lyricsH + 3 // subtract lyrics height + separator/header
		if viewH < 4 {
			viewH = 4
		}
	}

	var viewContent string
	switch a.view {
	case model.ViewHome:
		a.home.NeedsConfig = a.auth == nil
		a.home.Version = a.version
		viewContent = a.home.View(th, centerInnerW, viewH)
	case model.ViewLibrary:
		viewContent = a.library.View(th, centerInnerW, viewH)
	case model.ViewSearch:
		viewContent = a.search.View(th, centerInnerW, viewH)
	case model.ViewPlaylists:
		viewContent = a.playlist.View(th, centerInnerW, viewH)
	case model.ViewLyrics:
		viewContent = a.lyrics.View(th, centerInnerW, viewH, a.playback.Position.Milliseconds())
	case model.ViewSettings:
		viewContent = a.settings.View(th, a.config, centerInnerW, viewH)
	case model.ViewHelp:
		viewContent = a.help.View(th, centerInnerW, viewH)
	default:
		viewContent = lipgloss.NewStyle().
			Foreground(th.FgMuted).Italic(true).
				Render("  Coming soon...")
	}

	// Status message — cap width so a very long err.Error() from any
	// source can't blow out the center column. Full message is still in
	// the bridge log / stderr for debuggability.
	if a.status != "" {
		statusIcon := lipgloss.NewStyle().Foreground(th.Accent).Render("● ")
		maxW := centerInnerW - 4 // account for icon + padding
		if maxW < 20 {
			maxW = 20
		}
		trimmed := a.status
		if len([]rune(trimmed)) > maxW {
			runes := []rune(trimmed)
			trimmed = string(runes[:maxW-1]) + "…"
		}
		statusText := lipgloss.NewStyle().Foreground(th.FgDim).Render(trimmed)
		viewContent += "\n" + statusIcon + statusText
	}

	// Inline lyrics below content
	if inlineLyrics {
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
		// Reserve the last row for a key hints line so the user knows
		// the queue panel is interactive. Render tracks above it.
		trackLines := tlLines - 1
		if trackLines < 1 {
			trackLines = 1
		}
		startIdx := 0
		if a.queue.Index > trackLines/2 {
			startIdx = a.queue.Index - trackLines/2
		}
		for i := startIdx; i < a.queue.Len() && i < startIdx+trackLines; i++ {
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
		tlContent += "\n" + components.RenderHints(th, []components.Hint{
			{Key: "j/k · ↑↓", Desc: "move"},
			{Key: "Enter", Desc: "play"},
		})
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

	output := titleLine + "\n" + grid + "\n" + statusBar

	if a.modal.Active {
		modalBox := a.modal.View(th, a.width, a.height)
		output = components.Overlay(output, modalBox, a.width, a.height)
	}

	return output
}
