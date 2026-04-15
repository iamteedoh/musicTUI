// Package importbackend is the TUI-facing wrapper around the shared
// github.com/iamteedoh/musictui-import module. Everything runs
// locally — OAuth loopback, YT Data API calls, Spotify calls,
// matching, importing. No remote service.
//
// The package name is kept for historical continuity (v0.2.0 had a
// remote backend; internal/importbackend/ was the HTTP client for
// it) — messages.go and app.go references didn't need rewriting
// when the internals changed.
package importbackend

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/iamteedoh/musictui-import/importer"
	"github.com/iamteedoh/musictui-import/oauth"
	"github.com/iamteedoh/musictui-import/services/spotify"
	"github.com/iamteedoh/musictui-import/services/youtube"
	"github.com/iamteedoh/musictui-import/store"
)

// Client is the TUI-side handle into the shared import module.
// Holds the token store + per-service OAuth configs + lazily-built
// service clients.
type Client struct {
	store   *store.FileStore
	google  oauth.GoogleConfig
	spotify oauth.SpotifyConfig

	gmgr *oauth.GoogleTokenManager
	smgr *oauth.SpotifyTokenManager
}

// NewClient builds a Client rooted at `dir` for token storage.
// Callers pass the user-configured Google + Spotify OAuth client
// creds; if either is missing, methods that need them return an
// error telling the caller to run onboarding.
func NewClient(dir string, google oauth.GoogleConfig, spotify oauth.SpotifyConfig) (*Client, error) {
	st, err := store.NewFileStore(dir)
	if err != nil {
		return nil, err
	}
	c := &Client{
		store:   st,
		google:  google,
		spotify: spotify,
	}
	c.gmgr = &oauth.GoogleTokenManager{Store: st, Config: google}
	c.smgr = &oauth.SpotifyTokenManager{Store: st, Config: spotify}
	return c, nil
}

// ServicesStatus inspects the local store and reports which services
// have valid tokens.
type ServicesStatus struct {
	YouTube bool
	Spotify bool
}

// Services returns the current connection status by looking at the
// on-disk token store. Cheap — no network.
func (c *Client) Services() (ServicesStatus, error) {
	out := ServicesStatus{}
	yt, err := c.store.Load("youtube")
	if err != nil {
		return out, err
	}
	out.YouTube = yt != nil
	sp, err := c.store.Load("spotify")
	if err != nil {
		return out, err
	}
	out.Spotify = sp != nil
	return out, nil
}

// AuthYouTube runs the Google OAuth loopback flow end-to-end and
// persists the resulting token. Blocks until the browser redirects
// back or ctx times out.
func (c *Client) AuthYouTube(ctx context.Context) error {
	if c.google.ClientID == "" || c.google.ClientSecret == "" {
		return errors.New("Google OAuth credentials not configured — re-run onboarding")
	}
	lb, err := oauth.Listen(oauth.GoogleLoopbackPort, "")
	if err != nil {
		return err
	}
	verifier, challenge, err := oauth.PKCE()
	if err != nil {
		return err
	}
	state := oauth.State()
	authURL := oauth.GoogleAuthorizeURL(c.google, lb.URL, state, challenge)
	_ = openBrowser(authURL)

	res := lb.Wait(ctx)
	if res.Err != nil {
		return res.Err
	}
	if res.State != state {
		return errors.New("state nonce mismatch — possible CSRF")
	}
	tok, err := oauth.GoogleExchangeCode(ctx, c.google, lb.URL, res.Code, verifier)
	if err != nil {
		return fmt.Errorf("google token exchange: %w", err)
	}
	return c.store.Save("youtube", &oauth.ServiceToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	})
}

// AuthSpotify runs the Spotify OAuth loopback flow and persists the
// resulting token.
func (c *Client) AuthSpotify(ctx context.Context) error {
	if c.spotify.ClientID == "" || c.spotify.ClientSecret == "" {
		return errors.New("Spotify OAuth credentials not configured — re-run onboarding")
	}
	lb, err := oauth.Listen(oauth.SpotifyLoopbackPort, "")
	if err != nil {
		return err
	}
	state := oauth.State()
	authURL := oauth.SpotifyAuthorizeURL(c.spotify, lb.URL, state)
	_ = openBrowser(authURL)

	res := lb.Wait(ctx)
	if res.Err != nil {
		return res.Err
	}
	if res.State != state {
		return errors.New("state nonce mismatch — possible CSRF")
	}
	tok, err := oauth.SpotifyExchangeCode(ctx, c.spotify, lb.URL, res.Code)
	if err != nil {
		return fmt.Errorf("spotify token exchange: %w", err)
	}
	return c.store.Save("spotify", &oauth.ServiceToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	})
}

// LoadLibrary fetches playlists + liked count from YouTube.
func (c *Client) LoadLibrary(ctx context.Context) (*YouTubeLibrary, error) {
	yt := youtube.NewClient(c.gmgr.AccessToken)
	playlists, err := yt.ListPlaylists(ctx)
	if err != nil {
		return nil, err
	}
	// Liked count is informational; ignore errors (users commonly
	// have no liked videos and YT can 404 sporadically).
	liked, _ := yt.ListLikedVideos(ctx)

	out := &YouTubeLibrary{
		LikedCount: len(liked),
		Playlists:  make([]PlaylistSummary, 0, len(playlists)),
	}
	for _, p := range playlists {
		out.Playlists = append(out.Playlists, PlaylistSummary{
			ID:         p.ID,
			Name:       p.Name,
			TrackCount: p.TrackCount,
		})
	}
	return out, nil
}

// StartImport kicks the importer in a goroutine and returns the
// event channel immediately. Caller reads one event at a time
// (typically via a tea.Cmd loop).
func (c *Client) StartImport(ctx context.Context, req ImportRequest) <-chan importer.Event {
	yt := youtube.NewClient(c.gmgr.AccessToken)
	sp := spotify.NewClient(c.smgr.AccessToken)
	return importer.Run(ctx, yt, sp, importer.Request{
		Source:       req.Source,
		Dest:         req.Dest,
		PlaylistIDs:  req.PlaylistIDs,
		IncludeLiked: req.IncludeLiked,
	})
}

// ─────────────────── browser helper ───────────────────

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
