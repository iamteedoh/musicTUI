package ytmusic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client wraps an OAuth token and provides read access to YouTube Music
// library endpoints. Auto-refreshes the access token when it's close to
// expiry; persists refreshed tokens so subsequent launches don't need
// to re-auth.
type Client struct {
	tok  *Token
	http *http.Client
}

// NewClient returns a client bound to the given token. Caller should
// persist the token (via SaveToken) if it's newly minted.
func NewClient(tok *Token) *Client {
	return &Client{
		tok:  tok,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// ensureFresh refreshes the access token if it's expired or about to.
// Silently persists the refreshed token so the disk cache stays current.
func (c *Client) ensureFresh(ctx context.Context) error {
	if c.tok.Valid() {
		return nil
	}
	if c.tok.RefreshToken == "" {
		return fmt.Errorf("token expired and no refresh_token available")
	}
	fresh, err := RefreshToken(ctx, c.tok.RefreshToken)
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	c.tok = fresh
	_ = SaveToken(fresh)
	return nil
}

// UserInfo is the minimal subset of Google's userinfo response we use.
// Calling this is the cheapest way to verify that a token actually works
// end-to-end against Google's auth.
type UserInfo struct {
	Sub   string `json:"sub"`   // opaque user id
	Email string `json:"email"` // account email (if scope permits)
	Name  string `json:"name"`  // display name
}

// WhoAmI hits Google's userinfo endpoint. Used to verify auth succeeded
// without hitting any YT Music endpoint yet. Returns user info or an
// error if the token is invalid.
func (c *Client) WhoAmI(ctx context.Context) (*UserInfo, error) {
	if err := c.ensureFresh(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.tok.Authorization())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo: status %d", resp.StatusCode)
	}

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}
