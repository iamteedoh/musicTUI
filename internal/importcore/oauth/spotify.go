package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	spotifyAuthURL  = "https://accounts.spotify.com/authorize"
	spotifyTokenURL = "https://accounts.spotify.com/api/token"

	// Scopes for the import destination side. Reused with the user's
	// existing Spotify dev-app from onboarding (the same client_id
	// they use for playback — Spotify grants are per-scope, so the
	// first import triggers a fresh consent dialog for these broader
	// rights).
	SpotifyImportScopes = "playlist-modify-public playlist-modify-private playlist-read-private user-library-read"
)

// SpotifyConfig — one clientID reused across playback + import.
// ClientSecret is needed because Spotify's plain Authorization Code
// flow requires it at token exchange (we don't use PKCE here — we
// still send client_id + client_secret because the binary is
// trusted on the user's machine).
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
}

// SpotifyAuthorizeURL builds the consent URL. show_dialog=true
// forces a re-consent when the user re-runs auth (so they can
// switch accounts if desired) — a small UX nicety at no cost.
func SpotifyAuthorizeURL(cfg SpotifyConfig, redirectURI, state string) string {
	q := url.Values{
		"client_id":     []string{cfg.ClientID},
		"redirect_uri":  []string{redirectURI},
		"response_type": []string{"code"},
		"scope":         []string{SpotifyImportScopes},
		"state":         []string{state},
		"show_dialog":   []string{"true"},
	}
	return spotifyAuthURL + "?" + q.Encode()
}

// SpotifyTokenResponse is the decoded form of the /api/token reply.
type SpotifyTokenResponse struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	Scope        string
}

// SpotifyExchangeCode swaps an authorization_code for a token pair.
func SpotifyExchangeCode(
	ctx context.Context,
	cfg SpotifyConfig,
	redirectURI, code string,
) (*SpotifyTokenResponse, error) {
	form := url.Values{
		"grant_type":   []string{"authorization_code"},
		"code":         []string{code},
		"redirect_uri": []string{redirectURI},
	}
	return spotifyPostToken(ctx, cfg, form)
}

// SpotifyRefresh exchanges a refresh_token for a new access_token.
// Spotify *occasionally* rotates refresh_tokens (maybe 1% of
// requests); callers should persist a new one if present.
func SpotifyRefresh(ctx context.Context, cfg SpotifyConfig, refreshToken string) (*SpotifyTokenResponse, error) {
	form := url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
	}
	return spotifyPostToken(ctx, cfg, form)
}

func spotifyPostToken(ctx context.Context, cfg SpotifyConfig, form url.Values) (*SpotifyTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", spotifyTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// HTTP Basic (client_id:client_secret) — Spotify's docs lead with
	// this form. Works either way but Basic is less prone to quoting
	// weirdness than in-body creds.
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.ClientID + ":" + cfg.ClientSecret))
	req.Header.Set("Authorization", "Basic "+creds)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("spotify token: %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("spotify token parse: %w", err)
	}
	expires := time.Now().UTC().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return &SpotifyTokenResponse{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    expires,
		Scope:        parsed.Scope,
	}, nil
}
