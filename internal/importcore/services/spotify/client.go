// Package spotify wraps the Spotify Web API endpoints we need for
// the library-import destination side: whoami, track search, existing
// playlist inventory (for dedup), create playlist, add tracks.
//
// Gotchas baked in (learned during backend bring-up in v0.2.0):
//   - /search limit is 10 for apps in Development Mode (Nov 2024
//     change). Extended Quota Mode keeps 50. Defaulting to 10 keeps
//     self-hosters on fresh apps working.
//   - /playlists/{id}/items — not /tracks. Spotify renamed this Feb
//     2026. The old endpoint returns 403 even with correct scopes.
//   - /me/playlists — not /users/{id}/playlists. Avoids path-encoding
//     edge cases with auto-generated user IDs.
//   - add-tracks has a hard 100-URI per request limit.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iamteedoh/musicTUI/internal/importcore/match"
)

const (
	apiBase             = "https://api.spotify.com/v1"
	SearchLimit         = 10  // Dev Mode cap
	AddTracksBatchSize  = 100 // Spotify hard limit
	playlistNameMaxChar = 100
)

// APIError wraps a non-2xx response. Body is preserved for diagnosis.
type APIError struct {
	Status int
	Method string
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("spotify: %s %s -> %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// User is the authenticated Spotify user's profile summary.
type User struct {
	ID          string
	DisplayName string
	Email       string
}

// Playlist is a created/existing Spotify playlist.
type Playlist struct {
	ID   string
	Name string
	URL  string // open.spotify.com URL
}

// Client is the Spotify Web API wrapper. Safe across goroutines.
// All requests flow through a shared rate limiter so a single import
// can't hammer the API and blow through Dev Mode's limits.
type Client struct {
	http    *http.Client
	token   func(context.Context) (string, error)
	limiter *rateLimiter
}

// NewClient builds a Client. tokenFn returns a fresh access token
// per request (callers typically wire it to their token store).
func NewClient(tokenFn func(context.Context) (string, error)) *Client {
	return &Client{
		http:    &http.Client{},
		token:   tokenFn,
		limiter: newRateLimiter(),
	}
}

// rateLimiter enforces two guarantees:
//
//  1. Proactive pacing — no more than one request every `minInterval`
//     (stays safely under Dev Mode's per-app ~3 req/sec limit).
//  2. Respect 429s globally — when any request hits 429, ALL pending
//     and subsequent requests wait until Spotify's Retry-After
//     expires. Without this, parallel callers (search + add tracks)
//     would each hit 429 independently and multiply the total waits.
type rateLimiter struct {
	mu          sync.Mutex
	lastCall    time.Time
	minInterval time.Duration
	blockUntil  time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		// 300ms → ~3 req/sec. Under community-reported Dev Mode limits,
		// leaves headroom for Spotify's rolling-window accounting.
		minInterval: 300 * time.Millisecond,
	}
}

// Wait blocks until it's OK to make the next request, respecting
// both the pacing interval and any active 429 block.
func (r *rateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	now := time.Now()
	var wait time.Duration
	if now.Before(r.blockUntil) {
		wait = r.blockUntil.Sub(now)
	} else if since := now.Sub(r.lastCall); since < r.minInterval {
		wait = r.minInterval - since
	}
	// Set lastCall to the future instant the wait will end so other
	// concurrent callers space themselves after us.
	r.lastCall = now.Add(wait)
	r.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// BlockUntil records that Spotify told us to back off until `t`.
// Subsequent Wait() calls will sleep until then.
func (r *rateLimiter) BlockUntil(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t.After(r.blockUntil) {
		r.blockUntil = t
	}
}

// Whoami returns (id, display_name, email) for the authenticated user.
// Surface the display_name/email so error logs can pin a 403 Forbidden
// to a specific account (Dev Mode apps need users in the User
// Management list).
func (c *Client) Whoami(ctx context.Context) (User, error) {
	var body struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := c.do(ctx, "GET", "/me", nil, nil, &body); err != nil {
		return User{}, err
	}
	return User{ID: body.ID, DisplayName: body.DisplayName, Email: body.Email}, nil
}

// SearchTracks returns up to `limit` track candidates for `query`.
// Errors bubble up — the importer records them as per-track errors
// and moves on, so a single failed search doesn't abort the whole
// import.
func (c *Client) SearchTracks(ctx context.Context, query string, limit int) ([]match.Candidate, error) {
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > SearchLimit {
		limit = SearchLimit
	}
	params := url.Values{
		"q":     []string{query},
		"type":  []string{"track"},
		"limit": []string{strconv.Itoa(limit)},
	}
	var body struct {
		Tracks struct {
			Items []struct {
				URI   string `json:"uri"`
				ID    string `json:"id"`
				Name  string `json:"name"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
				Artists []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	if err := c.do(ctx, "GET", "/search", params, nil, &body); err != nil {
		return nil, err
	}
	out := make([]match.Candidate, 0, len(body.Tracks.Items))
	for _, t := range body.Tracks.Items {
		artists := make([]match.Artist, 0, len(t.Artists))
		for _, a := range t.Artists {
			artists = append(artists, match.Artist{Name: a.Name, ID: a.ID})
		}
		out = append(out, match.Candidate{
			URI:     t.URI,
			ID:      t.ID,
			Name:    t.Name,
			Album:   t.Album.Name,
			Artists: artists,
		})
	}
	return out, nil
}

// ListMyPlaylistNames returns the set of playlist names the current
// user owns or follows. Importer uses it to skip re-creating a
// playlist that already exists with the same name. Case-sensitive
// per Spotify's matching.
func (c *Client) ListMyPlaylistNames(ctx context.Context) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	offset := 0
	for {
		params := url.Values{
			"limit":  []string{"50"},
			"offset": []string{strconv.Itoa(offset)},
		}
		var body struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			Next string `json:"next"`
		}
		if err := c.do(ctx, "GET", "/me/playlists", params, nil, &body); err != nil {
			return nil, err
		}
		if len(body.Items) == 0 {
			return names, nil
		}
		for _, p := range body.Items {
			if p.Name != "" {
				names[p.Name] = struct{}{}
			}
		}
		if body.Next == "" {
			return names, nil
		}
		offset += len(body.Items)
	}
}

// CreatePlaylist makes a new playlist for the current user.
func (c *Client) CreatePlaylist(ctx context.Context, name, description string, public bool) (Playlist, error) {
	req := map[string]any{
		"name":        clampName(name),
		"description": description,
		"public":      public,
	}
	var body struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		ExternalURLs struct {
			Spotify string `json:"spotify"`
		} `json:"external_urls"`
	}
	if err := c.do(ctx, "POST", "/me/playlists", nil, req, &body); err != nil {
		return Playlist{}, err
	}
	return Playlist{
		ID:   body.ID,
		Name: body.Name,
		URL:  body.ExternalURLs.Spotify,
	}, nil
}

// AddTracks adds tracks to a playlist in batches of 100. Returns
// the count of URIs successfully submitted.
func (c *Client) AddTracks(ctx context.Context, playlistID string, uris []string) (int, error) {
	added := 0
	for i := 0; i < len(uris); i += AddTracksBatchSize {
		end := i + AddTracksBatchSize
		if end > len(uris) {
			end = len(uris)
		}
		chunk := uris[i:end]
		req := map[string]any{"uris": chunk}
		// /items, not /tracks — see package doc.
		if err := c.do(ctx, "POST", "/playlists/"+playlistID+"/items", nil, req, nil); err != nil {
			return added, err
		}
		added += len(chunk)
	}
	return added, nil
}

// ─────────────────── internals ───────────────────

// Retry policy for 429 responses. The rate limiter above prevents
// most 429s; these retries handle the rare ones that slip through.
// Capped at 2 attempts + 45s wait so a single bad track fails in
// under 2 minutes rather than stalling the whole import for ~10.
const (
	maxRetryAttempts = 2
	maxRetryWait     = 45 * time.Second
)

func (c *Client) do(
	ctx context.Context,
	method, path string,
	params url.Values,
	body any,
	out any,
) error {
	u := apiBase + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyBytes = b
	}

	for attempt := 0; attempt <= maxRetryAttempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}

		token, err := c.token(ctx)
		if err != nil {
			return fmt.Errorf("spotify token: %w", err)
		}

		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 {
			wait := parseRetryAfter(resp.Header.Get("Retry-After"))
			if wait <= 0 {
				wait = time.Duration(1<<attempt) * time.Second
			}
			// Mark the limiter so every other request waits too —
			// no point the importer firing 20 parallel calls that
			// all hit 429 while we're in a cooldown window.
			c.limiter.BlockUntil(time.Now().Add(wait))

			// Give up after the configured attempts OR if Spotify
			// asks for more than maxRetryWait. Waiting 2 minutes on
			// a single track with Dev-Mode throttling is worse UX
			// than failing fast and letting the user retry later.
			if attempt >= maxRetryAttempts || wait > maxRetryWait {
				// Lead with the decision and keep it short: the TUI shows
				// live errors on a single line, so the important part
				// (skipped + how long Spotify wanted) must come first.
				// The response body is just the 429 JSON — noise here.
				return &APIError{
					Status: resp.StatusCode, Method: method, Path: path,
					Body: fmt.Sprintf("rate-limited (retry-after %s) — track skipped, import continues", wait),
				}
			}
			continue
		}

		if resp.StatusCode >= 400 {
			return &APIError{Status: resp.StatusCode, Method: method, Path: path, Body: string(respBody)}
		}
		if out == nil || len(respBody) == 0 || resp.StatusCode == http.StatusNoContent {
			return nil
		}
		return json.Unmarshal(respBody, out)
	}
	return fmt.Errorf("spotify: %s %s still rate-limited after %d retries", method, path, maxRetryAttempts)
}

// parseRetryAfter returns the duration from a Retry-After header.
// Spotify sends seconds as a bare integer; HTTP also allows a date
// form but Spotify doesn't use it.
func parseRetryAfter(s string) time.Duration {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

func clampName(name string) string {
	name = strings.TrimSpace(name)
	if len(name) > playlistNameMaxChar {
		return name[:playlistNameMaxChar-1] + "…"
	}
	return name
}
