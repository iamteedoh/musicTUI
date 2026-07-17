package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	spotifylib "github.com/zmb3/spotify/v2"
	"golang.org/x/oauth2"

	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/importbackend"
	"github.com/iamteedoh/musicTUI/internal/importcore/importer"
	"github.com/iamteedoh/musicTUI/internal/importcore/oauth"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/mpris"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/termcap"
	"github.com/iamteedoh/musicTUI/internal/theme"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
	"github.com/iamteedoh/musicTUI/internal/update"
)

type App struct {
	config config.Config
	theme  theme.Theme
	// termBg is the terminal's own background color ("#rrggbb") from the
	// startup probe, empty when it didn't answer. Kept so switching the
	// Theme setting back to Auto can re-resolve against the real background
	// without another probe round-trip.
	termBg               string
	width                int
	height               int
	focus                model.FocusMode
	view                 model.View
	showLyrics           bool // toggle inline lyrics in center panel (default: true)
	sidebarPlaylistFocus bool // true when sidebar focus is on the playlist section
	cleanupPromptShown   bool // true once the duplicate/empty cleanup prompt has been offered this session
	modal                components.Modal
	onboard              components.Onboard
	help                 components.Help
	prevView             model.View // view to return to when ? help is closed
	sidebar              components.Sidebar
	home                 components.Home
	library              components.Library
	search               components.Search
	playlist             components.Playlists
	playing              components.NowPlaying
	viz                  *components.MiniVisualizer
	lyrics               components.Lyrics
	// Pointer (like viz): Artwork holds a mutex and, in hi-res mode, mutates
	// its kitty-graphics bookkeeping during View — state that must survive
	// Bubble Tea's per-frame copies of App.
	artwork            *components.Artwork
	settings           components.Settings
	importv            components.Import
	importsetup        components.ImportSetup
	importClient       *importbackend.Client
	importEvents       <-chan importer.Event // active import event stream; nil when no import is running
	importEventsCancel context.CancelFunc    // cancels the importer goroutine on view exit
	playback           model.PlaybackState
	queue              model.Queue
	status             string

	// viewCache memoizes the expensive lipgloss panel renders (borders/margins)
	// for regions that don't change every frame. Pointer so it persists across
	// View's value receiver (App is copied each frame). See (*App).View.
	cache *viewCache

	// out serializes frames and graphics payloads onto one terminal writer.
	// nil in tests, where the raw-write fallback is used instead.
	out *TermWriter

	// sixelEncoding guards against piling up encode commands: only one cover
	// is ever in flight, and a resize supersedes it rather than queueing.
	sixelEncoding bool

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

	// authWaitCancel cancels the in-flight browser-login callback wait (and
	// its :8888 HTTP server). Held so we can abort a stuck login — e.g. when
	// the Client ID is wrong and Spotify never redirects back — before it
	// blocks a fresh attempt on the same port. nil when no wait is active.
	authWaitCancel context.CancelFunc
}

func NewApp(cfg config.Config, bridgePath string, version string) App {
	// One probe answers everything we need from the terminal: kitty graphics,
	// sixel graphics, the pixel size of a cell (which sixel needs in order to
	// land on cell boundaries), and the background color that auto-theming
	// matches a palette against (MUS-32). The probe no-ops on a non-TTY, so
	// tests and piped runs stay side-effect-free.
	caps := termcap.Detect()
	th := theme.Resolve(cfg.Theme, caps.Bg)
	app := App{
		config:      cfg,
		theme:       th,
		termBg:      caps.Bg,
		version:     version,
		focus:       model.FocusSidebar,
		view:        model.ViewHome,
		sidebar:     components.NewSidebar(),
		home:        components.NewHome(),
		onboard:     components.NewOnboard(),
		help:        components.NewHelp(),
		library:     components.NewLibrary(),
		search:      components.NewSearch(),
		playlist:    components.NewPlaylists(),
		playing:     components.NewNowPlaying(),
		viz:         components.NewMiniVisualizer(),
		cache:       &viewCache{},
		lyrics:      components.NewLyrics(),
		settings:    components.NewSettings(),
		importv:     components.NewImport(),
		importsetup: components.NewImportSetup(),
		showLyrics:  true,
		playback:    model.PlaybackState{Volume: cfg.Volume},
		queue:       model.NewQueue(),
		bridgePath:  bridgePath,
	}
	// Pixel-perfect album art where the terminal supports kitty-graphics
	// Unicode placeholders (kitty, Ghostty), sixel (Windows Terminal,
	// Konsole, …) or iTerm2's native inline images; error-minimized block
	// art elsewhere (MUSICTUI_ARTWORK=blocks|braille|kitty|sixel|iterm2
	// overrides). The terminal is queried directly for support (reliable
	// across platforms, unlike env sniffing which missed Ghostty on Linux —
	// MUS-20), except iTerm2, which answers the kitty query without
	// rendering placeholders and is identified by name instead (MUS-30).
	// Terminals that answer nothing get character art.
	art := components.NewArtwork()
	app.artwork = &art
	app.artwork.SetStyle(components.DetectArtworkStyle(caps.Kitty, caps.Sixel))
	app.artwork.SetCellSize(caps.CellW, caps.CellH)
	if cfg.Spotify.ClientID != "" {
		app.auth = sp.NewAuth(cfg.Spotify.ClientID)
	} else {
		// First launch with no credentials — walk the user through setup.
		app.onboard.Start()
	}
	// Set up the embedded import client. Runs fully locally against
	// the user's own Google Cloud + Spotify OAuth apps — no hosted
	// service. Tokens live under the musicTUI config dir.
	importDir, _ := config.ConfigDir()
	if importDir != "" {
		importDir += "/import"
	}
	spotifyID := cfg.SpotifyImportClientID()
	importReady := cfg.Import.GoogleClientID != "" &&
		cfg.Import.GoogleClientSecret != "" &&
		spotifyID != "" &&
		cfg.Import.SpotifyClientSecret != ""
	if importReady {
		importClient, err := importbackend.NewClient(
			importDir,
			oauth.GoogleConfig{
				ClientID:     cfg.Import.GoogleClientID,
				ClientSecret: cfg.Import.GoogleClientSecret,
			},
			oauth.SpotifyConfig{
				ClientID:     spotifyID,
				ClientSecret: cfg.Import.SpotifyClientSecret,
			},
		)
		if err == nil {
			app.importClient = importClient
		}
	}
	if app.importClient == nil {
		app.importv.MarkNotConfigured()
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

// 60 fps render tick. The visualizer is tuned for this rate (its CAVA timing
// and the audio-output delay compensation were dialed in at 60 fps); the
// per-frame cost is kept low by precomputing the visualizer's per-cell colors
// (see miniviz rebuild) rather than styling each cell every frame.
// SetOutput gives the app the same writer Bubble Tea renders through, so
// graphics payloads can be sequenced against frames rather than racing them.
// Call before handing the model to tea.NewProgram.
func (a *App) SetOutput(w *TermWriter) { a.out = w }

// writeRawCmd writes escape sequences directly to the terminal, bypassing
// Bubble Tea's renderer. Fallback for when no TermWriter is set (tests);
// carries the same invisible control data that must not be diffed, cached, or
// truncated the way view content is.
func writeRawCmd(seq string) tea.Cmd {
	return func() tea.Msg {
		_, _ = os.Stdout.WriteString(seq)
		return nil
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
			// The app-owner-Premium 403 isn't a stale token — keep the cached
			// credentials (re-auth won't fix it) and surface the real cause.
			if errors.Is(err, sp.ErrAppOwnerNotPremium) {
				return AppOwnerNotPremiumMsg{}
			}
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
// appendImportLog writes a timestamped line to
// ~/.config/musicTUI/import/last-run.log so we can diagnose import
// failures after the fact (TUI alone can only show a few summary
// lines; the full log has everything). Best-effort — if we can't
// write, we silently drop.
func appendImportLog(format string, args ...any) {
	dir, err := config.ConfigDir()
	if err != nil {
		return
	}
	path := filepath.Join(dir, "import", "last-run.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	line := time.Now().UTC().Format("2006-01-02T15:04:05Z") + " " +
		fmt.Sprintf(format, args...) + "\n"
	_, _ = f.WriteString(line)
}

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

// errAuthTimeout is returned when the browser login never redirects back
// within the callback window. The most common cause is a wrong/rejected
// Spotify Client ID (Spotify shows "Invalid client id" on its own page and
// never hits our callback), so the message points at the in-app fix.
var errAuthTimeout = errors.New(`login timed out — if the browser showed "Invalid client id", press Ctrl+O to re-enter your Spotify Client ID`)

func (a App) waitForAuthCallbackCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		tok, err := a.auth.WaitForCallback(ctx)
		if err != nil {
			switch {
			case errors.Is(err, context.Canceled):
				// Wait was aborted deliberately (e.g. Ctrl+O to re-enter the
				// Client ID, or a fresh login superseding this one). Not an
				// error — stay quiet so we don't clobber the new state.
				return nil
			case errors.Is(err, context.DeadlineExceeded):
				return AuthErrorMsg{Err: errAuthTimeout}
			default:
				return AuthErrorMsg{Err: fmt.Errorf("login failed: %w", err)}
			}
		}

		httpClient := a.auth.HTTPClient(tok)
		client := sp.NewClient(spotifylib.New(httpClient), httpClient)
		username, err := client.FetchUsername(context.Background())
		if err != nil {
			if errors.Is(err, sp.ErrAppOwnerNotPremium) {
				return AppOwnerNotPremiumMsg{}
			}
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
		if a.artwork.UsesSixel() {
			// A resize moves the artwork panel, so the cover is repainted at new
			// coordinates. In some terminals — Konsole — writing text over sixel
			// pixels does not erase them, so the old copy is orphaned wherever it
			// was and shows through the tracklist. Only an explicit screen erase
			// removes it (MUS-29).
			//
			// The erase must land BEFORE the cover is repainted, or we paint it
			// and wipe it in the same breath — which is why the repaint is
			// sequenced after the clear rather than done here.
			return a, tea.Sequence(tea.ClearScreen, sixelRepaintCmd())
		}
		// A resize makes Bubble Tea repaint every line unconditionally, which
		// the row diff can't observe. Force the cover to be painted again —
		// and re-render the panel itself, because a cursor-positioned draw
		// (iTerm2) bakes in the panel origin: a resize can move the panel
		// without changing its size, and a memo hit would republish the old
		// coordinates. (Sixel takes the branch above, whose repaint message
		// drops the memo for the same reason.)
		a.cache.art = panelMemo{}
		a.cache.artRows = ""
		return a, nil

	case sixelRepaintMsg:
		// The screen has just been erased. Drop the panel memo and the cached
		// rows so the next frame re-renders the artwork and paints the cover.
		a.cache.art = panelMemo{}
		a.cache.artRows = ""
		return a, nil
	case TickMsg:
		a.viz.SetPosition(a.playback.Position.Milliseconds())
		a.viz.Update(a.playback.IsPlaying)
		// Hand queued graphics escapes (kitty transmit/placement, or a sixel
		// image) to the terminal writer, which paints them directly after the
		// next frame. Writing them from a command goroutine instead would race
		// Bubble Tea's renderer: the payload could tear, or land before the
		// frame that blanks its cells and be erased by it.
		if oob := a.artwork.TakeOOB(); oob != "" {
			if a.out != nil {
				a.out.Queue(oob)
			} else {
				return a, tea.Batch(tickCmd(), writeRawCmd(oob))
			}
		}
		if a.out != nil {
			// A frame identical to the last one is never written, so on a
			// static screen a queued image would wait forever. After 100ms of
			// no drawing, the blanks are certainly already on screen.
			a.out.FlushStale(100 * time.Millisecond)
		}
		// Encoding a cover costs tens of milliseconds, so it happens in a
		// command rather than in View. One at a time: a resize drag supersedes
		// the pending geometry faster than we could ever encode it.
		if !a.sixelEncoding {
			if w, ok := a.artwork.PendingSixel(); ok {
				a.sixelEncoding = true
				return a, tea.Batch(tickCmd(), EncodeSixelCmd(w))
			}
		}
		return a, tickCmd()

	case SixelEncodedMsg:
		a.sixelEncoding = false
		if a.artwork.SetSixelPayload(msg.URL, msg.Cols, msg.Rows, msg.Payload) {
			// The panel renders to the same blank cells either way, so its memo
			// still hits and renderSixel would never run to publish the draw.
			// Drop the memo, and the cached rows with it, so the newly encoded
			// cover is actually painted.
			a.cache.art = panelMemo{}
			a.cache.artRows = ""
		}
		return a, nil

	// Auth
	case AuthURLMsg:
		a.status = "Login URL opened — complete auth in browser"
		a.home.AuthURL = msg.URL
		a.home.AppOwnerNotPremium = false
		// Cancel any previous callback wait so its :8888 server is released
		// before we bind a new one (rapid re-auth, or Ctrl+O recovery).
		if a.authWaitCancel != nil {
			a.authWaitCancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		a.authWaitCancel = cancel
		return a, a.waitForAuthCallbackCmd(ctx)
	case AuthSuccessMsg:
		a.client = msg.Client
		a.home.Username = msg.Username
		a.home.AuthURL = ""
		a.home.AppOwnerNotPremium = false
		if a.authWaitCancel != nil {
			a.authWaitCancel()
			a.authWaitCancel = nil
		}
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
		if a.authWaitCancel != nil {
			a.authWaitCancel()
			a.authWaitCancel = nil
		}
		// The login is over (failed/timed out), so stop showing the
		// "waiting in browser" URL block. errAuthTimeout is already
		// self-contained and actionable, so show it verbatim; prefix
		// other auth errors.
		a.home.AuthURL = ""
		if errors.Is(msg.Err, errAuthTimeout) {
			a.status = msg.Err.Error()
		} else {
			a.status = "Auth error: " + msg.Err.Error()
		}
		return a, nil
	case AppOwnerNotPremiumMsg:
		// Login worked but Spotify is blocking every API call because the
		// Developer app's owner lacks Premium. Re-auth won't help, so render
		// actionable recovery steps in the Home view instead of a raw error.
		a.pendingRetryTrack = nil
		a.client = nil
		a.home.Username = ""
		a.home.AuthURL = ""
		a.home.AppOwnerNotPremium = true
		a.status = "Spotify blocked the app: its owner needs Premium — see Home for how to fix"
		return a, nil

	// Import (v0.3.0 — embedded, runs locally)
	case ServicesStatusMsg:
		a.importv.YouTubeConnected = msg.Status.YouTube
		a.importv.SpotifyConnected = msg.Status.Spotify
		if msg.Status.YouTube && msg.Status.Spotify {
			a.importv.Stage = components.ImportStageLoadingLibrary
			return a, LoadLibraryCmd(a.importClient)
		}
		// Auto-trigger OAuth for the next service that needs it.
		next := a.importv.NextServiceToConnect()
		if next != "" && a.importv.AuthBrowserOpenedFor != next {
			a.importv.AuthBrowserOpenedFor = next
			a.importv.Stage = components.ImportStageAwaitingAuth
			return a, AuthServiceCmd(a.importClient, next)
		}
		a.importv.Stage = components.ImportStageAwaitingAuth
		return a, nil
	case ServicesStatusErrorMsg:
		a.importv.Stage = components.ImportStageError
		a.importv.Err = msg.Err
		return a, nil
	case ServiceAuthedMsg:
		// Just landed one OAuth flow — re-check status so the next
		// service auto-triggers (or library load kicks off).
		a.importv.AuthBrowserOpenedFor = ""
		return a, CheckServicesCmd(a.importClient)
	case ServiceAuthErrorMsg:
		a.importv.Stage = components.ImportStageError
		a.importv.Err = fmt.Errorf("%s auth: %w", msg.Service, msg.Err)
		a.importv.AuthBrowserOpenedFor = ""
		return a, nil
	case ImportLibraryLoadedMsg:
		a.importv.Stage = components.ImportStageLibraryLoaded
		a.importv.Playlists = msg.Library.Playlists
		a.importv.LikedCount = msg.Library.LikedCount
		return a, nil
	case ImportLibraryErrorMsg:
		a.importv.Stage = components.ImportStageError
		a.importv.Err = msg.Err
		return a, nil
	case StartImportMsg:
		a.importv.Stage = components.ImportStageImporting
		a.importEvents = msg.Events
		a.importEventsCancel = msg.Cancel
		return a, ListenImportEventCmd(msg.Events)
	case ImportEventMsg:
		ev := msg.Event
		switch ev.Type {
		case importer.EventJobStarted:
			// nothing user-visible — the importing view is already showing
		case importer.EventPlaylistSkipped:
			a.importv.ProgressOverallTotal++
		case importer.EventPlaylistStarted:
			a.importv.ProgressCurrentPlaylist = ev.PlaylistName
			a.importv.ProgressDone = 0
			a.importv.ProgressTotal = ev.PlaylistTotal
			a.importv.ProgressOverall++
			if a.importv.ProgressOverall > a.importv.ProgressOverallTotal {
				a.importv.ProgressOverallTotal = a.importv.ProgressOverall
			}
		case importer.EventTrackMatched:
			a.importv.ProgressDone = ev.TrackIndex
			a.importv.ProgressTotal = ev.PlaylistTotal
			a.importv.Matched++
		case importer.EventTrackUnmatched:
			a.importv.ProgressDone = ev.TrackIndex
			a.importv.ProgressTotal = ev.PlaylistTotal
			if strings.HasPrefix(ev.TrackReason, "search failed") {
				a.importv.Errors++
				if a.importv.ErrorReasons == nil {
					a.importv.ErrorReasons = map[string]int{}
				}
				// Strip the "search failed: " prefix for display, leaving
				// just the underlying error (e.g. the Spotify 429 body).
				reason := strings.TrimPrefix(ev.TrackReason, "search failed: ")
				a.importv.ErrorReasons[reason]++
				appendImportLog("search failed for %s — %s: %s",
					ev.TrackTitle, ev.TrackArtist, reason)
			} else {
				a.importv.Unmatched++
			}
		case importer.EventPlaylistDone:
			a.importv.JobURL = ev.PlaylistURL
		case importer.EventJobDone:
			a.importv.Stage = components.ImportStageDone
			a.importv.ProgressOverallTotal = ev.PlaylistCount
			if a.importEventsCancel != nil {
				a.importEventsCancel()
				a.importEventsCancel = nil
			}
			a.importEvents = nil
			if a.client != nil {
				a.playlist.Items = nil
				return a, FetchPlaylistsCmd(a.client, 0)
			}
			return a, nil
		case importer.EventError:
			// The importer emits EventError for both fatal and
			// non-fatal failures (bad create, bad add, bad whoami,
			// etc.). A fatal one closes the event stream right
			// after; a non-fatal one keeps going. We record the
			// message regardless so the Done screen shows it, and
			// stash it on Err so ImportStreamClosedMsg can surface
			// the actual reason if the job does terminate.
			if a.importv.ErrorReasons == nil {
				a.importv.ErrorReasons = map[string]int{}
			}
			a.importv.ErrorReasons[ev.Message]++
			a.importv.Errors++
			a.importv.Err = fmt.Errorf("%s", ev.Message)
			appendImportLog("error: %s", ev.Message)
		}
		return a, ListenImportEventCmd(a.importEvents)
	case ImportStreamClosedMsg:
		if a.importv.Stage == components.ImportStageImporting {
			a.importv.Stage = components.ImportStageError
			// Preserve the last EventError we saw (set by the case
			// above) — that's the real reason the worker bailed. If
			// we never received one, fall back to a generic message.
			if a.importv.Err == nil {
				a.importv.Err = fmt.Errorf("import event stream closed unexpectedly")
			}
		}
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
		// A fresh fetch always starts at offset 0. Reset the list on the first
		// page so a re-fetch (e.g. the playlist reload after a stale-token
		// re-auth, which happens when a track finishes and the next one comes
		// back Unavailable) REPLACES the list instead of appending a second
		// copy of every playlist. The old append-only behavior doubled the
		// in-memory list, which then tripped the "duplicate playlists" prompt
		// mid-playback and looked like the playlists had been duplicated
		// (MUS-13). Later pages (offset > 0) still append, for pagination.
		if msg.Offset == 0 {
			a.playlist.Items = nil
		}
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
		// Advertise the restore key only when a backup actually exists.
		a.playlist.RestoreAvailable = sp.HasBackups()

		// Opt-in cleanup prompts. Disabled by default because these
		// actions unfollow playlists (Spotify's API has no true delete),
		// and unfollowing a playlist you own removes it from /me/playlists
		// with no public API to list it back. Explicit warning in the
		// prompt copy.
		// Only offer the cleanup prompts once per session, on the first full
		// load — never on the background re-fetches that follow a re-auth.
		// Popping a confirmation modal in the middle of playback (which is
		// what happened in MUS-13) is jarring and unexpected.
		if a.config.CheckDuplicates && !a.cleanupPromptShown {
			a.cleanupPromptShown = true
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
					"Extras will be unfollowed (not deleted). A backup is saved first — " +
					"press R afterwards to restore, or use Spotify's 90-day recovery page."
				a.modal.ShowConfirm("Merge Duplicate Playlists", dupMsg, components.ActionConsolidateDuplicates, "")
				return a, nil
			}
			if empty := a.findEmptyPlaylists(); len(empty) > 0 {
				emptyMsg := fmt.Sprintf("%d empty playlist(s) found:\n\n", len(empty))
				for _, pl := range empty {
					emptyMsg += fmt.Sprintf("  \"%s\"\n", pl.Name)
				}
				emptyMsg += "\nRemove them from your library?\n" +
					"These will be unfollowed (not deleted). A backup is saved first — " +
					"press R afterwards to restore, or use Spotify's 90-day recovery page."
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
		a.status = fmt.Sprintf("Consolidated %d group(s), removed %d playlist(s) — press R to restore", msg.MergedCount, msg.DeletedCount)
		if a.client != nil {
			return a, FetchPlaylistsCmd(a.client, 0)
		}
		return a, nil
	case EmptyPlaylistsDeletedMsg:
		a.playlist.Items = nil
		a.playlist.Total = 0
		a.playlist.Loading = true
		a.status = fmt.Sprintf("Deleted %d empty playlist(s) — press R to restore", msg.DeletedCount)
		if a.client != nil {
			return a, FetchPlaylistsCmd(a.client, 0)
		}
		return a, nil
	case PlaylistsRestoredMsg:
		a.playlist.Items = nil
		a.playlist.Total = 0
		a.playlist.Loading = true
		a.status = fmt.Sprintf("Restored %d playlist(s) (%d re-followed, %d recreated, %d failed)",
			msg.Refollowed+msg.Recreated, msg.Refollowed, msg.Recreated, msg.Failed)
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
	// Import setup wizard takes over while active (similar full-screen
	// takeover as Onboard).
	if a.importsetup.Active {
		return a.handleImportSetupKey(msg)
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
	case "ctrl+o":
		// In-app recovery for a wrong/rejected Spotify Client ID (MUS-12):
		// abort any stuck login wait (freeing :8888) and reopen the setup
		// wizard on the Client ID step, pre-filled, so the user can paste a
		// correct one. Finishing the wizard saves it, rebuilds auth, and
		// re-fires login via the existing onboarding path.
		if a.authWaitCancel != nil {
			a.authWaitCancel()
			a.authWaitCancel = nil
		}
		a.home.AuthURL = ""
		a.status = ""
		a.onboard.StartAtClientID(a.config.Spotify.ClientID)
		// The wizard opens straight on the paste step, so send the user to
		// where the Client ID lives at the same time.
		openBrowser("https://developer.spotify.com/dashboard")
		return a, nil
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

	// Arrow keys for panel switching (unless typing in search). The focused
	// Settings panel keeps ←/→ for itself — they cycle the selected value
	// there (MUS-32) — so panel switching skips that one case; Esc/h/Tab
	// still leave the panel.
	settingsOwnsKeys := a.view == model.ViewSettings && a.focus == model.FocusContent
	if !a.isSearchInputFocused() && !settingsOwnsKeys {
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

	// Playback keys (unless typing in search). The Import screen is
	// excluded: it owns letters that collide with playback bindings —
	// notably r (reconnect service vs cycle repeat) — and this switch
	// returns early, so the Import error screen's "r: reconnect" hint
	// silently cycled repeat mode instead of reconnecting.
	if !a.isSearchInputFocused() && a.view != model.ViewImport {
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
			// The focused Settings panel uses l (vim-right) to cycle the
			// selected value; everywhere else it toggles inline lyrics.
			if settingsOwnsKeys {
				break
			}
			a.showLyrics = !a.showLyrics
			return a, nil
		}
	}

	// Some views take over the entire center panel and should receive
	// keys regardless of whether the sidebar is focused — otherwise
	// reaching them via the sidebar (where focus stays on Sidebar)
	// leaves Esc and j/k unrouted.
	switch a.view {
	case model.ViewHelp, model.ViewImport:
		return a.handleContentKey(msg)
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
	case "left":
		a.onboard.Prev()
		return a, nil
	case "h":
		// Vim-style back, but only where there's no text field to type into —
		// on the paste-Client-ID step 'h' is a character, not a navigation key.
		if !a.onboard.OnFinalStep() {
			a.onboard.Prev()
			return a, nil
		}
	}

	if a.onboard.OnFinalStep() {
		switch s {
		case "enter":
			clientID := a.onboard.ClientID()
			if clientID == "" {
				a.onboard.Error = "Client ID can't be empty"
				return a, nil
			}
			oldPlaybackID := a.config.Spotify.ClientID
			a.config.Spotify.ClientID = clientID
			if err := config.Save(a.config); err != nil {
				a.onboard.Error = "Failed to save config: " + err.Error()
				return a, nil
			}
			// The import's "reuse playback app" path borrows this client
			// id. If the id changed, the stored import token was issued
			// under the OLD app and can only fail with invalid_client on
			// refresh — drop it and rebuild the import client against the
			// new app. (The import wizard guards its own saves the same
			// way; this is the other place the effective id can change.)
			if oldPlaybackID != clientID && a.config.Import.SpotifyClientID == "" && a.importClient != nil {
				if dir, _ := config.ConfigDir(); dir != "" {
					dir += "/import"
					_ = os.Remove(filepath.Join(dir, "spotify.json"))
					if c, err := importbackend.NewClient(
						dir,
						oauth.GoogleConfig{ClientID: a.config.Import.GoogleClientID, ClientSecret: a.config.Import.GoogleClientSecret},
						oauth.SpotifyConfig{ClientID: clientID, ClientSecret: a.config.Import.SpotifyClientSecret},
					); err == nil {
						a.importClient = c
					}
				}
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
			for _, r := range typedRunes(msg) {
				a.onboard.InputChar(r)
			}
			return a, nil
		}
	}

	// Non-final steps: advance on enter / right, open browser on 'o'.
	switch s {
	case "enter", "right", "l":
		a.onboard.Next()
		if a.onboard.OnFinalStep() {
			// Landing on the paste-Client-ID step — take the user straight
			// to where the Client ID lives so they don't have to hunt.
			openBrowser("https://developer.spotify.com/dashboard")
		}
		return a, nil
	case "o", "O":
		if a.onboard.Step == 1 {
			openBrowser("https://developer.spotify.com/dashboard")
		}
		return a, nil
	}
	return a, nil
}

func (a App) handleImportSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := msg.String()

	switch s {
	case "ctrl+c":
		return a, tea.Quit
	case "esc":
		a.importsetup.Close()
		return a, nil
	case "ctrl+u":
		// Clear the field under the cursor — essential for replacing a
		// masked secret you can't read.
		a.importsetup.ClearField()
		return a, nil
	case "left", "h":
		if !a.importsetup.IsInputStep() {
			a.importsetup.Prev()
			return a, nil
		}
	}

	// Final "Done" step: Enter saves + closes.
	if a.importsetup.IsFinalStep() && s == "enter" {
		gID, gSecret, sClientID, sSecret := a.importsetup.Trimmed()
		if !a.importsetup.Complete() {
			a.importsetup.Error = "All required fields must be filled — go back and fill them in."
			return a, nil
		}
		// Detect app-switch so we can invalidate the stored Spotify
		// token (tied to the old client_id, useless once the app
		// changes). Comparing effective client_ids: what we were
		// using vs what we will now use.
		oldSpotifyID := a.config.SpotifyImportClientID()
		a.config.Import.GoogleClientID = gID
		a.config.Import.GoogleClientSecret = gSecret
		a.config.Import.SpotifyClientID = sClientID // empty when reusing playback app
		a.config.Import.SpotifyClientSecret = sSecret
		newSpotifyID := a.config.SpotifyImportClientID()

		if err := config.Save(a.config); err != nil {
			a.importsetup.Error = "Failed to save config: " + err.Error()
			return a, nil
		}
		dir, _ := config.ConfigDir()
		if dir != "" {
			dir += "/import"
		}
		// If the Spotify app changed, drop the old token file so
		// AuthSpotify re-runs against the new app next time.
		if oldSpotifyID != newSpotifyID {
			_ = os.Remove(filepath.Join(dir, "spotify.json"))
		}
		client, err := importbackend.NewClient(
			dir,
			oauth.GoogleConfig{ClientID: gID, ClientSecret: gSecret},
			oauth.SpotifyConfig{ClientID: newSpotifyID, ClientSecret: sSecret},
		)
		if err != nil {
			a.importsetup.Error = "Couldn't initialise import client: " + err.Error()
			return a, nil
		}
		a.importClient = client
		a.importv = components.NewImport()
		a.importsetup.Close()
		return a, nil
	}

	// Choice step (Spotify app strategy): j/k to move, Enter to confirm.
	if a.importsetup.IsChoiceStep() {
		switch s {
		case "j", "down":
			a.importsetup.CycleChoice(+1)
			return a, nil
		case "k", "up":
			a.importsetup.CycleChoice(-1)
			return a, nil
		case "enter", "right", "l":
			a.importsetup.Error = ""
			a.importsetup.Next()
			return a, nil
		}
		return a, nil
	}

	// Input steps: type into the active field.
	if a.importsetup.IsInputStep() {
		switch s {
		case "tab":
			a.importsetup.SwitchField()
			return a, nil
		case "enter":
			// Validate this step's fields before advancing.
			if a.importsetup.Step == 5 {
				if g, gs, _, _ := a.importsetup.Trimmed(); g == "" || gs == "" {
					a.importsetup.Error = "Both Client ID and Client Secret are required."
					return a, nil
				}
			} else if a.importsetup.Step == 8 {
				_, _, sID, ss := a.importsetup.Trimmed()
				if ss == "" {
					a.importsetup.Error = "Spotify Client Secret is required."
					return a, nil
				}
				if a.importsetup.SpotifyUseDedicated && sID == "" {
					a.importsetup.Error = "Spotify Client ID is required for the dedicated app."
					return a, nil
				}
			}
			a.importsetup.Error = ""
			a.importsetup.Next()
			return a, nil
		case "backspace":
			a.importsetup.Backspace()
			return a, nil
		default:
			rs := typedRunes(msg)
			if len(rs) > 1 {
				a.importsetup.Paste(string(rs))
				return a, nil
			}
			if len(s) == 1 && s[0] >= 32 && s[0] < 127 {
				a.importsetup.InputChar(rune(s[0]))
				return a, nil
			}
			for _, r := range rs {
				a.importsetup.InputChar(r)
			}
			return a, nil
		}
	}

	// Non-input, non-choice, non-final steps: navigation + browser-open.
	switch s {
	case "enter", "right", "l":
		a.importsetup.Error = ""
		a.importsetup.Next()
		return a, nil
	case "o", "O":
		if url := a.importsetup.URLForStep(); url != "" {
			openBrowser(url)
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
			case components.ActionRestorePlaylists:
				a.modal.Close()
				a.status = "Restoring playlists from backup..."
				if a.client != nil {
					return a, RestorePlaylistsCmd(a.client)
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
			} else {
				for _, r := range typedRunes(msg) {
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
	case model.ViewImport:
		switch msg.String() {
		case "enter":
			// Enter on the sidebar moves focus to content; Enter on
			// content runs the import state machine.
			if a.focus == model.FocusSidebar {
				a.focus = model.FocusContent
				return a, nil
			}
			return a.handleImportEnter()
		case "r":
			if a.importv.Stage == components.ImportStageError && a.importClient != nil {
				advice := components.ImportErrorAdviceFor(a.importv.Err)
				if advice.Service != "" {
					a.importv.Stage = components.ImportStageAwaitingAuth
					a.importv.AuthBrowserOpenedFor = advice.Service
					a.importv.Err = nil
					if advice.Service == "youtube" {
						a.importv.YouTubeConnected = false
					}
					if advice.Service == "spotify" {
						a.importv.SpotifyConnected = false
					}
					return a, ReauthServiceCmd(a.importClient, advice.Service)
				}
			}
			// Retry the OAuth flow for whichever service still needs auth.
			if a.importv.Stage == components.ImportStageAwaitingAuth && a.importClient != nil {
				if next := a.importv.NextServiceToConnect(); next != "" {
					a.importv.AuthBrowserOpenedFor = next
					return a, AuthServiceCmd(a.importClient, next)
				}
			}
		case "esc", "h":
			// Cancel any active SSE stream when leaving the view so the
			// goroutine cleans up — events can resume on re-entry via
			// the cached session + GET /jobs/{id} fallback.
			if a.importEventsCancel != nil {
				a.importEventsCancel()
				a.importEventsCancel = nil
				a.importEvents = nil
			}
			a.focus = model.FocusSidebar
		case "j", "down", "k", "up":
			// No in-view list to scroll through — delegate to the
			// sidebar so the user can still move to other views. This
			// is important because handleKey special-routes ViewImport
			// to handleContentKey even when focus is on the sidebar.
			return a.handleNavKey(msg)
		case "c":
			// Re-open the setup wizard. Useful for switching Spotify
			// strategy (shared → dedicated) after hitting rate limits,
			// or re-pasting a rotated client secret.
			a.importsetup.Start(
				a.config.Import.GoogleClientID,
				a.config.Import.GoogleClientSecret,
				a.config.Import.SpotifyClientID,
				a.config.Import.SpotifyClientSecret,
			)
			return a, nil
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
		// Enter and →/l step a setting forward, ← steps it back; a boolean
		// just flips either way. h keeps meaning "back to sidebar", so only
		// the arrow pair (and l, vim's right) cycles values.
		case "enter", "l", "right":
			a.changeSetting(1)
		case "left":
			a.changeSetting(-1)
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

// changeSetting applies a value change to the selected settings row and
// persists it: booleans flip, the theme cycles delta steps.
func (a *App) changeSetting(delta int) {
	switch a.settings.SelectedKey() {
	case "theme":
		a.cycleTheme(delta)
	case "check_duplicates":
		a.config.CheckDuplicates = !a.config.CheckDuplicates
		_ = config.Save(a.config)
	}
}

// cycleTheme steps the Theme setting through theme.Options() (Auto, then
// every built-in dark → medium → light), persists the choice, and re-resolves
// the active theme immediately. The View fingerprint keys off the theme, so
// every memoized panel repaints on the next frame — the switch is live, no
// restart. Auto re-resolves against the background captured at startup.
func (a *App) cycleTheme(delta int) {
	opts := theme.Options()
	cur := a.config.Theme
	if cur == "" {
		cur = theme.Auto
	}
	idx := 0 // unknown names (config typos) restart the cycle at Auto
	for i, k := range opts {
		if k == cur {
			idx = i
			break
		}
	}
	idx = ((idx+delta)%len(opts) + len(opts)) % len(opts)
	a.config.Theme = opts[idx]
	_ = config.Save(a.config)
	a.theme = theme.Resolve(a.config.Theme, a.termBg)
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
		} else {
			for _, r := range typedRunes(msg) {
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
	case "R":
		// Restore playlists removed by a previous merge/cleanup, using the
		// most recent on-disk backup.
		bf, path, err := sp.LoadLatestBackup()
		if err != nil {
			a.status = "No playlist backup found to restore from"
			return a, nil
		}
		msg := fmt.Sprintf(
			"Restore %d playlist(s) from backup?\n\n"+
				"Backup taken: %s\nReason: %s\nFile: %s\n\n"+
				"Each playlist is re-followed by its original ID, or recreated\n"+
				"from the backup if it no longer exists on Spotify.",
			len(bf.Playlists), bf.CreatedAt, bf.Reason, path)
		a.modal.ShowConfirm("Restore Playlists", msg, components.ActionRestorePlaylists, "")
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

// handleImportEnter drives the Import view's state machine on Enter
// presses. v0.3.0 flow: everything runs locally via the embedded
// importbackend client. Idle → EnsuringSession (just a status
// check) → AwaitingAuth (per-service loopback OAuth, sequential)
// → LoadingLibrary → LibraryLoaded → Importing → Done/Error.
func (a App) handleImportEnter() (tea.Model, tea.Cmd) {
	// If the user hits Enter on the "not configured" screen, launch
	// the import setup wizard rather than dropping into an error.
	if a.importv.Stage == components.ImportStageNotConfigured ||
		a.importClient == nil {
		a.importsetup.Start(
			a.config.Import.GoogleClientID,
			a.config.Import.GoogleClientSecret,
			a.config.Import.SpotifyClientID,
			a.config.Import.SpotifyClientSecret,
		)
		return a, nil
	}
	switch a.importv.Stage {
	case components.ImportStageIdle:
		a.importv.Stage = components.ImportStageEnsuringSession
		return a, CheckServicesCmd(a.importClient)
	case components.ImportStageError:
		if a.importEventsCancel != nil {
			a.importEventsCancel()
			a.importEventsCancel = nil
			a.importEvents = nil
		}
		a.importv.Reset()
		return a, nil
	case components.ImportStageLibraryLoaded:
		return a, StartImportCmd(a.importClient, false /* include_liked */)
	case components.ImportStageDone:
		a.importv.ProgressOverall = 0
		a.importv.ProgressOverallTotal = 0
		a.importv.ProgressDone = 0
		a.importv.ProgressTotal = 0
		a.importv.Matched = 0
		a.importv.Unmatched = 0
		a.importv.Errors = 0
		a.importv.JobURL = ""
		a.importv.Stage = components.ImportStageLoadingLibrary
		return a, LoadLibraryCmd(a.importClient)
	}
	return a, nil
}

func (a App) viewTitle() string {
	switch a.view {
	case model.ViewHome:
		return "HOME"
	case model.ViewLibrary:
		return "LIKED SONGS"
	case model.ViewSearch:
		return "SEARCH"
	case model.ViewPlaylists:
		return "PLAYLISTS"
	case model.ViewVisualizer:
		return "VISUALIZER"
	case model.ViewLyrics:
		return "LYRICS"
	case model.ViewImport:
		return "IMPORT"
	case model.ViewHelp:
		return "HELP"
	case model.ViewSettings:
		return "SETTINGS"
	default:
		return "musicTUI"
	}
}

// panelMemo caches one panel's rendered string, keyed by a cheap content/geometry
// signature. The key encodes everything that affects the render, so a hit can
// never return stale output — a change to any input changes the key.
type panelMemo struct{ key, val string }

func (m *panelMemo) get(key string, build func() string) string {
	if m.key == key && m.val != "" {
		return m.val
	}
	v := build()
	m.key, m.val = key, v
	return v
}

// viewCache holds the memo slots for the panels that are static (or near-static)
// during playback. The visualizer panel is deliberately NOT cached — it renders
// live every frame.
type viewCache struct {
	left   panelMemo
	center panelMemo
	title  panelMemo
	art    panelMemo

	// artRows is the last frame's content of the screen rows the sixel cover
	// occupies. Bubble Tea rewrites a whole line when any column on it changes,
	// which erases the pixels — so when these rows differ, the image must be
	// painted again.
	artRows string
}

func (a App) View() string {
	if a.width == 0 || a.height == 0 {
		return ""
	}

	if a.onboard.Active {
		return a.onboard.View(a.theme, a.width, a.height)
	}
	if a.importsetup.Active {
		return a.importsetup.View(a.theme, a.width, a.height)
	}

	th := a.theme
	bc := th.Border // all inner panels use this border color

	// Theme fingerprint — prefixed into every cache key so switching themes
	// invalidates all memoized panels. Name alone distinguishes the built-in
	// themes; the color fields keep the key honest if names ever collide.
	tf := th.Name + "/" + string(th.Border) + "/" + string(th.BorderFocused) + "/" +
		string(th.Accent) + "/" + string(th.Surface) + "/" + string(th.FgDim)

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
	leftKey := tf + "|" + string(leftBc) + "|" +
		strconv.Itoa(leftW) + "x" + strconv.Itoa(gridH) + "|" +
		strconv.Itoa(navLines) + "/" + strconv.Itoa(plLines) + "|" +
		navContent + "\x1e" + plContent
	leftCol := a.cache.left.get(leftKey, func() string {
		return components.MultiSectionColumn(
			[]components.PanelSection{
				{Title: "NAVIGATION", Content: navContent, Lines: navLines},
				{Title: "PLAYLISTS", Content: plContent, Lines: plLines},
			},
			leftW, gridH, leftBc, th.Surface, th,
		)
	})

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

	// Quantize the displayed position to 250ms so the center panel's content is
	// byte-identical most frames (it then reuses its cached border render rather
	// than rebuilding 60x/sec for a sub-character progress change). The live
	// 1:28/5:26 clock is in the status bar, which is NOT cached. Lyrics highlight
	// at 250ms resolution — imperceptible for line-synced lyrics.
	qPlayback := a.playback
	qPlayback.Position = a.playback.Position.Truncate(250 * time.Millisecond)
	qMs := qPlayback.Position.Milliseconds()

	// Build center panel content: now-playing info at top + current view below
	var centerContent string

	// Now-playing header (if playing)
	if a.playback.Track != nil {
		npContent := a.playing.PanelContent(th, qPlayback, centerInnerW)
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
		viewContent = a.lyrics.View(th, centerInnerW, viewH, qMs)
	case model.ViewSettings:
		viewContent = a.settings.View(th, a.config, centerInnerW, viewH)
	case model.ViewHelp:
		viewContent = a.help.View(th, centerInnerW, viewH)
	case model.ViewImport:
		viewContent = a.importv.View(th, centerInnerW, viewH)
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
		lyricsContent := a.lyrics.View(th, centerInnerW, lyricsH, qMs)
		viewContent += lyricsSep + lyricsHeader + lyricsContent
	}

	centerTitle := "NOW PLAYING"
	if a.playback.Track == nil {
		centerTitle = a.viewTitle()
	}
	centerContent += viewContent
	centerKey := tf + "|" + string(centerBc) + "|" +
		strconv.Itoa(centerW) + "x" + strconv.Itoa(gridH) + "|" +
		centerTitle + "\x1e" + centerContent
	centerPanel := a.cache.center.get(centerKey, func() string {
		return components.TitledPanel(centerTitle, centerContent, centerW, gridH, centerBc, th)
	})

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

	// Where the ARTWORK section's content area starts on screen, 1-based. The
	// sixel renderer paints at the cursor, so it needs absolute coordinates;
	// every other style ignores this. Rows: title(1) + column top border(1) +
	// tracklist(tlLines) + ARTWORK divider(1). Cols: left + center columns,
	// then the column's own "│" border.
	a.artwork.SetOrigin(leftW+centerW+2, tlLines+4)

	// Artwork content — cached on the artwork's signature so the per-cell dot
	// matrix isn't re-rendered every frame (the cover is static within a track;
	// the signature changes when the art loads or the track changes).
	//
	// The modal state is part of the key because a modal painting over the
	// artwork destroys a sixel image; dismissing it must re-render the panel so
	// the pixels are drawn again.
	artKey := tf + "|" + strconv.Itoa(rightW-2) + "x" + strconv.Itoa(artLines) + "|" +
		strconv.FormatBool(a.modal.Active) + "|" + a.artwork.Signature()
	artContent := a.cache.art.get(artKey, func() string {
		return a.artwork.View(th, rightW-2, artLines)
	})

	// Visualizer: animated bars (sets the live BPM estimate as a side effect)
	vizContent := a.viz.View(th, rightW-2, vizLines)
	vizTitle := "VISUALIZER"
	if bpm := a.viz.LastBPM(); bpm > 0 && a.playback.IsPlaying {
		vizTitle = fmt.Sprintf("VISUALIZER · %d BPM", bpm)
	}

	rightCol := components.MultiSectionColumn(
		[]components.PanelSection{
			{Title: "TRACKLIST", Content: tlContent, Lines: tlLines},
			{Title: "ARTWORK", Content: artContent, Lines: artLines},
			{Title: vizTitle, Content: vizContent, Lines: vizLines},
		},
		rightW, gridH, rightBc, th.Surface, th,
	)

	// ══════════════════════════════════════════════════════════════
	// ASSEMBLE: title + 3-column grid + status bar
	// ══════════════════════════════════════════════════════════════
	grid := lipgloss.JoinHorizontal(lipgloss.Top, leftCol, centerPanel, rightCol)

	// Title line: ─── musicTUI v0.3.0 | Terminal Music Player | ● username ───
	// The version is always visible here so a user can tell at a glance which
	// build they're running (MUS-20) — the same string `musicTUI --version`
	// prints.
	titleStr := "musicTUI"
	if a.version != "" {
		titleStr += " " + a.version
	}
	titleStr += " | Terminal Music Player"
	if a.home.Username != "" {
		titleStr += " | ● " + a.home.Username
	}
	titleKey := tf + "|" + strconv.Itoa(a.width) + "|" + titleStr
	titleLine := a.cache.title.get(titleKey, func() string {
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
		return borderStyle.Render(strings.Repeat("─", titleDashL)) + " " +
			titleText + " " +
			borderStyle.Render(strings.Repeat("─", titleDashR))
	})

	// Status bar (full width)
	statusBar := a.playing.StatusBarView(th, a.playback, a.width)

	output := titleLine + "\n" + grid + "\n" + statusBar

	if a.modal.Active {
		modalBox := a.modal.View(th, a.width, a.height)
		output = components.Overlay(output, modalBox, a.width, a.height)
	}

	a.repaintSixelIfClobbered(output)

	return output
}

// repaintSixelIfClobbered re-queues the cover whenever Bubble Tea is about to
// rewrite a line the image sits on.
//
// The renderer diffs by whole line, and JoinHorizontal merges the three columns
// into one. So a changing track position in the center column rewrites the full
// line — artwork cells included — and erases the pixels on it. The rows below,
// whose neighbours happened to be static, survive: the cover appears sliced in
// half (MUS-29).
//
// Queuing here rather than from Update is deliberate: View runs immediately
// before the renderer writes this frame, so TermWriter paints the image onto
// cells the terminal has just blanked.
func (a App) repaintSixelIfClobbered(frame string) {
	if a.out == nil {
		return
	}
	if a.modal.Active {
		// A modal covers the artwork; painting over it would be wrong. Forget
		// the rows so that dismissing it — which restores their previous
		// content byte for byte — still counts as a change and repaints.
		a.cache.artRows = ""
		return
	}
	seq, row, rows := a.artwork.SixelDraw()
	if seq == "" || rows <= 0 {
		return
	}
	lines := strings.Split(frame, "\n")
	lo, hi := row-1, row-1+rows
	if lo < 0 || hi > len(lines) {
		return // layout disagrees with the placement; don't paint blind
	}
	covered := strings.Join(lines[lo:hi], "\n")
	if covered == a.cache.artRows {
		return // those rows are unchanged, so the pixels are still intact
	}
	a.cache.artRows = covered
	// Atomic: this frame blanks the very cells the payload paints, so the two
	// must render as one or the cover blinks on every lyric (MUS-30).
	a.out.QueueAtomic(seq)
}
