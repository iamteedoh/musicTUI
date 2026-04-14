// Package ytmusic implements a minimal client for YouTube Music's internal
// API, used for reading a user's library so it can be imported into
// Spotify. Auth uses Google's OAuth 2.0 device-flow with the publicly-
// known "YouTube TV" client credentials (same approach ytmusicapi uses —
// these credentials are bundled in every smart-TV YouTube app and are
// not secrets in any meaningful sense).
//
// This flow keeps the user-facing setup to a single one-time "open a URL
// and enter a short code" step — no Google Cloud project creation, no
// OAuth credential wrangling.
package ytmusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Public client credentials shipped with every YouTube TV surface. Used
// by ytmusicapi, yt-dlp, and many other FOSS projects. Included in the
// binary is fine — these are widely-distributed public identifiers, not
// secrets. A real client secret in Google's sense isn't required for
// the device flow on public clients.
const (
	ytTVClientID     = "861556708454-d6dlm3lh05idd8npek18k6be8ba3oc68.apps.googleusercontent.com"
	ytTVClientSecret = "SboVhoG9s0rNafixCSGGKXAT"
	deviceCodeURL    = "https://oauth2.googleapis.com/device/code"
	tokenURL         = "https://oauth2.googleapis.com/token"
	scope            = "https://www.googleapis.com/auth/youtube"
)

// DeviceAuth is the response from the device-code endpoint.
type DeviceAuth struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// Token is a Google OAuth token with the fields we care about.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int       `json:"expires_in,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
}

// Valid reports whether the token has a non-expired access token.
func (t *Token) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.Expiry.Add(-30*time.Second))
}

// Authorization string for HTTP headers.
func (t *Token) Authorization() string {
	return t.TokenType + " " + t.AccessToken
}

// RequestDeviceCode kicks off the device flow, returning the user_code
// the user enters on verification_url. Caller displays these to the
// user, then polls with PollForToken.
func RequestDeviceCode(ctx context.Context) (*DeviceAuth, error) {
	form := url.Values{}
	form.Set("client_id", ytTVClientID)
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deviceCodeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code: status %d", resp.StatusCode)
	}

	var da DeviceAuth
	if err := json.NewDecoder(resp.Body).Decode(&da); err != nil {
		return nil, err
	}
	return &da, nil
}

// PollForToken polls Google's token endpoint until the user approves
// the device authorization or the device code expires. Uses the poll
// interval the server asks us to. Respects the caller-supplied context.
func PollForToken(ctx context.Context, deviceCode string, interval int) (*Token, error) {
	if interval < 1 {
		interval = 5
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			tok, err := exchangeDeviceCode(ctx, deviceCode)
			if err != nil {
				if errors.Is(err, errAuthPending) || errors.Is(err, errSlowDown) {
					// slow_down means the server is asking us to back off
					if errors.Is(err, errSlowDown) {
						interval += 5
						ticker.Reset(time.Duration(interval) * time.Second)
					}
					continue
				}
				return nil, err
			}
			return tok, nil
		}
	}
}

var (
	errAuthPending = errors.New("authorization_pending")
	errSlowDown    = errors.New("slow_down")
)

func exchangeDeviceCode(ctx context.Context, deviceCode string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", ytTVClientID)
	form.Set("client_secret", ytTVClientSecret)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	switch body.Error {
	case "":
		// success
	case "authorization_pending":
		return nil, errAuthPending
	case "slow_down":
		return nil, errSlowDown
	case "access_denied":
		return nil, errors.New("authorization denied by user")
	case "expired_token":
		return nil, errors.New("device code expired; please retry")
	default:
		return nil, fmt.Errorf("token exchange: %s", body.Error)
	}

	return &Token{
		AccessToken:  body.AccessToken,
		RefreshToken: body.RefreshToken,
		ExpiresIn:    body.ExpiresIn,
		TokenType:    body.TokenType,
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}, nil
}

// RefreshToken exchanges a refresh token for a new access token. Used
// transparently when the in-memory token expires.
func RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	form := url.Values{}
	form.Set("client_id", ytTVClientID)
	form.Set("client_secret", ytTVClientSecret)
	form.Set("refresh_token", refreshToken)
	form.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh: status %d", resp.StatusCode)
	}

	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	return &Token{
		AccessToken:  body.AccessToken,
		RefreshToken: refreshToken, // refresh stays the same
		ExpiresIn:    body.ExpiresIn,
		TokenType:    body.TokenType,
		Expiry:       time.Now().Add(time.Duration(body.ExpiresIn) * time.Second),
	}, nil
}

// Persistence — stored next to Spotify credentials so both services
// share one credentials directory per user.

// TokenPath returns the file path where the cached YT Music token
// lives. Same parent directory as config.toml / credentials.json.
func TokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "musicTUI", "ytmusic-credentials.json"), nil
}

// SaveToken writes the token to disk with 0600 permissions so other
// users on the host can't read it.
func SaveToken(t *Token) error {
	p, err := TokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(t)
}

// LoadToken reads a cached token, if present. Returns nil with no error
// when the file doesn't exist — callers should treat that as "no auth".
func LoadToken() (*Token, error) {
	p, err := TokenPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var t Token
	if err := json.NewDecoder(f).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ClearToken removes the cached token, used when the user signs out
// or the refresh flow definitively fails.
func ClearToken() error {
	p, err := TokenPath()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
