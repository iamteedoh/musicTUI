package applemusic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// apiBase is Apple Music's public API endpoint. Every library call
// needs both the Developer Token (as Authorization) and the Music
// User Token (as Music-User-Token header).
const apiBase = "https://api.music.apple.com/v1"

// Client wraps the two-token Apple Music auth pattern.
type Client struct {
	creds *Credentials
	http  *http.Client
}

func NewClient(c *Credentials) *Client {
	return &Client{
		creds: c,
		http:  &http.Client{Timeout: 30 * time.Second},
	}
}

// get performs an authenticated GET against the Apple Music API and
// decodes the JSON into `out`. Query parameters go in `query`.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := apiBase + path
	if query != nil {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.creds.DeveloperToken)
	req.Header.Set("Music-User-Token", c.creds.MusicUserToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apple music: %s %d: %s", path, resp.StatusCode, truncate(string(b), 200))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// WhoAmI — Apple Music's equivalent minimal auth-check. Uses the
// storefront endpoint which every valid credentials pair can hit.
// Returns the user's storefront ID (country code) on success.
func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/me/storefront", nil, &body); err != nil {
		return "", err
	}
	if len(body.Data) == 0 {
		return "", fmt.Errorf("empty storefront response")
	}
	return body.Data[0].ID, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
