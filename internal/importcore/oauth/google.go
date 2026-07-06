package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"

	// Scope needed to read a user's YT library via YT Data API v3.
	// youtube.readonly is a *sensitive* scope — but we're using the
	// "user brings their own OAuth app" model, so each user is the
	// sole test user of their own app and no OAuth consent screen
	// verification is needed.
	GoogleYouTubeReadonlyScope = "https://www.googleapis.com/auth/youtube.readonly"
)

// GoogleConfig is the user-supplied OAuth client credentials from
// Google Cloud Console → APIs & Services → Credentials.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
}

// GoogleAuthorizeURL builds the URL to send the user's browser to
// for the initial consent. Uses access_type=offline + prompt=consent
// to reliably get a refresh_token (Google only returns it on the
// first consent unless prompt=consent forces it).
func GoogleAuthorizeURL(cfg GoogleConfig, redirectURI, state, codeChallenge string) string {
	q := url.Values{
		"client_id":             []string{cfg.ClientID},
		"redirect_uri":          []string{redirectURI},
		"response_type":         []string{"code"},
		"scope":                 []string{GoogleYouTubeReadonlyScope},
		"access_type":           []string{"offline"},
		"prompt":                []string{"consent"},
		"state":                 []string{state},
		"code_challenge":        []string{codeChallenge},
		"code_challenge_method": []string{"S256"},
	}
	return googleAuthURL + "?" + q.Encode()
}

// GoogleTokenResponse is what the token endpoint returns on both
// authorization_code and refresh_token grants.
type GoogleTokenResponse struct {
	AccessToken  string
	RefreshToken string // only present on first exchange (or if Google rotates, rare)
	ExpiresAt    time.Time
	Scope        string
}

// GoogleExchangeCode swaps an authorization_code for a token pair.
// PKCE verifier is mandatory on our auth URL; Google requires it
// here too.
func GoogleExchangeCode(
	ctx context.Context,
	cfg GoogleConfig,
	redirectURI, code, codeVerifier string,
) (*GoogleTokenResponse, error) {
	form := url.Values{
		"grant_type":    []string{"authorization_code"},
		"code":          []string{code},
		"redirect_uri":  []string{redirectURI},
		"client_id":     []string{cfg.ClientID},
		"client_secret": []string{cfg.ClientSecret},
		"code_verifier": []string{codeVerifier},
	}
	return googlePostToken(ctx, form)
}

// GoogleRefresh uses a stored refresh_token to get a fresh
// access_token. Google *may* return a new refresh_token (rare);
// callers should persist it if present.
func GoogleRefresh(ctx context.Context, cfg GoogleConfig, refreshToken string) (*GoogleTokenResponse, error) {
	form := url.Values{
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
		"client_id":     []string{cfg.ClientID},
		"client_secret": []string{cfg.ClientSecret},
	}
	return googlePostToken(ctx, form)
}

func googlePostToken(ctx context.Context, form url.Values) (*GoogleTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google token: %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("google token parse: %w", err)
	}
	expires := time.Now().UTC().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return &GoogleTokenResponse{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		ExpiresAt:    expires,
		Scope:        parsed.Scope,
	}, nil
}
