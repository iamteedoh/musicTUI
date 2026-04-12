package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"

	"github.com/iamteedoh/musicTUI/internal/config"
)

const (
	redirectURI  = "http://127.0.0.1:8888/callback"
	callbackPort = "8888"
)

var requiredScopes = []string{
	spotifyauth.ScopeUserReadPrivate,
	spotifyauth.ScopeUserReadEmail,
	spotifyauth.ScopeUserLibraryRead,
	spotifyauth.ScopeUserLibraryModify,
	spotifyauth.ScopePlaylistReadPrivate,
	spotifyauth.ScopePlaylistReadCollaborative,
	spotifyauth.ScopePlaylistModifyPublic,
	spotifyauth.ScopePlaylistModifyPrivate,
	spotifyauth.ScopeStreaming,
	spotifyauth.ScopeUserReadPlaybackState,
	spotifyauth.ScopeUserModifyPlaybackState,
	spotifyauth.ScopeUserReadRecentlyPlayed,
}

// Auth manages Spotify OAuth PKCE flow and token caching.
type Auth struct {
	clientID     string
	auth         *spotifyauth.Authenticator
	codeVerifier string
	state        string
	mu           sync.Mutex
}

func NewAuth(clientID string) *Auth {
	a := spotifyauth.New(
		spotifyauth.WithClientID(clientID),
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(requiredScopes...),
	)

	return &Auth{
		clientID: clientID,
		auth:     a,
	}
}

func generateRandomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}

func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// AuthURL generates the Spotify authorization URL for PKCE flow.
func (a *Auth) AuthURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.codeVerifier = generateRandomString(128)
	a.state = generateRandomString(16)
	challenge := generateCodeChallenge(a.codeVerifier)

	return a.auth.AuthURL(a.state,
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("client_id", a.clientID),
	)
}

// WaitForCallback starts a local HTTP server and waits for the OAuth callback.
func (a *Auth) WaitForCallback(ctx context.Context) (*oauth2.Token, error) {
	a.mu.Lock()
	verifier := a.codeVerifier
	state := a.state
	a.mu.Unlock()

	tokenCh := make(chan *oauth2.Token, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    ":" + callbackPort,
		Handler: mux,
	}

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		tok, err := a.auth.Token(r.Context(), state, r,
			oauth2.SetAuthURLParam("code_verifier", verifier),
		)
		if err != nil {
			http.Error(w, "Auth failed", http.StatusForbidden)
			errCh <- fmt.Errorf("token exchange failed: %w", err)
			return
		}
		if st := r.FormValue("state"); st != state {
			http.Error(w, "State mismatch", http.StatusForbidden)
			errCh <- fmt.Errorf("state mismatch")
			return
		}

		// Save token immediately
		_ = SaveToken(tok)

		fmt.Fprintf(w, `<html><body><h2>Login Successful!</h2><p>You can close this window and return to musicTUI.</p></body></html>`)
		tokenCh <- tok
	})

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	defer func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutCtx)
	}()

	select {
	case tok := <-tokenCh:
		return tok, nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cachedTokenJSON matches the format written by the Rust rspotify version.
type cachedTokenJSON struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	// Go oauth2 format uses "expiry" instead of "expires_at"
	Expiry string `json:"expiry,omitempty"`
}

// CachedToken loads a cached token from disk, handling both Rust and Go formats.
func CachedToken() (*oauth2.Token, error) {
	path, err := config.CredentialsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw cachedTokenJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	tok := &oauth2.Token{
		AccessToken:  raw.AccessToken,
		TokenType:    raw.TokenType,
		RefreshToken: raw.RefreshToken,
	}

	// Parse expiry from whichever field is present
	if raw.Expiry != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw.Expiry); err == nil {
			tok.Expiry = t
		}
	} else if raw.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw.ExpiresAt); err == nil {
			tok.Expiry = t
		}
	} else if raw.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	}

	if tok.TokenType == "" {
		tok.TokenType = "Bearer"
	}

	return tok, nil
}

// ClearToken removes the cached token from disk.
func ClearToken() {
	path, err := config.CredentialsPath()
	if err != nil {
		return
	}
	_ = os.Remove(path)
}

// SaveToken caches a token to disk in Go oauth2 format.
func SaveToken(tok *oauth2.Token) error {
	path, err := config.CredentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// HTTPClient returns an *http.Client that auto-refreshes tokens.
func (a *Auth) HTTPClient(tok *oauth2.Token) *http.Client {
	return a.auth.Client(context.Background(), tok)
}
