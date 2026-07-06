package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// skewSeconds: refresh if the access_token is within this many
// seconds of expiry. Avoids racy 401s on in-flight requests.
const skewSeconds = 60

// ServiceToken is the stored-on-disk token shape. Both Google and
// Spotify use the same shape.
type ServiceToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

// Store is the interface a credential store must satisfy. The
// default implementation (store/creds.go) persists to disk with
// symmetric encryption.
type Store interface {
	Load(service string) (*ServiceToken, error)
	Save(service string, tok *ServiceToken) error
	Delete(service string) error
}

// ─────────────────── Google refresher ───────────────────

// GoogleTokenManager wraps a Store + GoogleConfig to provide an
// AccessToken(ctx) method that refreshes on the fly.
type GoogleTokenManager struct {
	Store  Store
	Config GoogleConfig

	mu sync.Mutex // serialize refresh to avoid double-spending a refresh
}

// AccessToken returns a live Google access token, refreshing if the
// stored one is within skewSeconds of expiry. Persists the refreshed
// token back to the Store.
func (m *GoogleTokenManager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, err := m.Store.Load("youtube")
	if err != nil {
		return "", err
	}
	if tok == nil {
		return "", fmt.Errorf("no youtube token — run auth")
	}
	if !isExpiring(tok.ExpiresAt) {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("youtube token expired and no refresh_token — re-auth required")
	}
	refreshed, err := GoogleRefresh(ctx, m.Config, tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("google refresh: %w", err)
	}
	tok.AccessToken = refreshed.AccessToken
	if refreshed.RefreshToken != "" { // Google rarely rotates these
		tok.RefreshToken = refreshed.RefreshToken
	}
	tok.ExpiresAt = refreshed.ExpiresAt
	if refreshed.Scope != "" {
		tok.Scope = refreshed.Scope
	}
	if err := m.Store.Save("youtube", tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// ─────────────────── Spotify refresher ───────────────────

type SpotifyTokenManager struct {
	Store  Store
	Config SpotifyConfig

	mu sync.Mutex
}

// AccessToken returns a live Spotify access token, refreshing if
// needed. Spotify sometimes rotates the refresh_token — we persist
// it when that happens.
func (m *SpotifyTokenManager) AccessToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tok, err := m.Store.Load("spotify")
	if err != nil {
		return "", err
	}
	if tok == nil {
		return "", fmt.Errorf("no spotify token — run auth")
	}
	if !isExpiring(tok.ExpiresAt) {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", fmt.Errorf("spotify token expired and no refresh_token — re-auth required")
	}
	refreshed, err := SpotifyRefresh(ctx, m.Config, tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("spotify refresh: %w", err)
	}
	tok.AccessToken = refreshed.AccessToken
	if refreshed.RefreshToken != "" {
		tok.RefreshToken = refreshed.RefreshToken
	}
	tok.ExpiresAt = refreshed.ExpiresAt
	if refreshed.Scope != "" {
		tok.Scope = refreshed.Scope
	}
	if err := m.Store.Save("spotify", tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

// ─────────────────── shared ───────────────────

func isExpiring(expiresAt time.Time) bool {
	if expiresAt.IsZero() {
		return false // no expiry info — assume valid
	}
	return expiresAt.Before(time.Now().UTC().Add(time.Duration(skewSeconds) * time.Second))
}
