package importbackend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBackendURL is the public hosted backend. Self-hosters override
// via [import_backend] url = ... in config.toml.
const DefaultBackendURL = "https://musictui-import.iamteedoh.dev"

// Client wraps the HTTP surface of musictui-import. One per app
// session; safe to share across goroutines.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a Client with a sensible timeout. Pass an empty
// string to use DefaultBackendURL.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBackendURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Long timeout — library reads against YT Data API can take
		// a bit for libraries with many playlists. SSE uses a
		// separate, untimed client (see ListenEvents).
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

// BackendURL returns the configured base URL — used to build the
// browser-side OAuth start URLs.
func (c *Client) BackendURL() string { return c.baseURL }

// AuthStartURL is the URL the CLI hands to the user's default browser
// to begin OAuth for `service` ("youtube" or "spotify"). The backend
// 302s to Google/Spotify and back to its own callback; the browser
// finally lands on a "you can close this tab" page.
func (c *Client) AuthStartURL(service, sessionID string) string {
	return fmt.Sprintf(
		"%s/auth/%s/start?session=%s",
		c.baseURL, service, url.QueryEscape(sessionID),
	)
}

// CreateSession mints a fresh session on the backend and returns the
// credentials the CLI should persist.
func (c *Client) CreateSession(ctx context.Context) (*Session, error) {
	var resp struct {
		SessionID string `json:"session_id"`
		CSRFToken string `json:"csrf_token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := c.do(ctx, "POST", "/api/session", nil, "", &resp); err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &Session{
		SessionID:  resp.SessionID,
		CSRFToken:  resp.CSRFToken,
		ExpiresAt:  resp.ExpiresAt,
		BackendURL: c.baseURL,
	}, nil
}

// GetSessionState fetches the session's current OAuth-connection
// state. Returns nil session and 404 if the backend forgot us
// (expired or rotated DB).
func (c *Client) GetSessionState(ctx context.Context, sessionID string) (*SessionState, error) {
	var s SessionState
	if err := c.do(ctx, "GET", "/api/session/"+url.PathEscape(sessionID), nil, "", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// LoadYouTubeLibrary fetches playlists + liked count for the session.
// Backend returns 400 if YouTube isn't connected — surface that
// distinctly so the UI can prompt for re-auth.
func (c *Client) LoadYouTubeLibrary(ctx context.Context, sessionID string) (*YouTubeLibrary, error) {
	var lib YouTubeLibrary
	path := "/api/session/" + url.PathEscape(sessionID) + "/library/youtube"
	if err := c.do(ctx, "GET", path, nil, "", &lib); err != nil {
		return nil, err
	}
	return &lib, nil
}

// StartImport kicks an import job and returns the job_id. CSRF token
// is required by the backend on this mutation.
func (c *Client) StartImport(
	ctx context.Context, sessionID, csrfToken string, req ImportRequest,
) (string, error) {
	var resp StartImportResponse
	path := "/api/session/" + url.PathEscape(sessionID) + "/import"
	if err := c.do(ctx, "POST", path, req, csrfToken, &resp); err != nil {
		return "", err
	}
	return resp.JobID, nil
}

// GetJob fetches the final/current state of a job — used as a fallback
// when SSE drops or after re-launch to recover an in-flight import.
func (c *Client) GetJob(ctx context.Context, sessionID, jobID string) (*JobState, error) {
	var st JobState
	path := "/api/session/" + url.PathEscape(sessionID) + "/jobs/" + url.PathEscape(jobID)
	if err := c.do(ctx, "GET", path, nil, "", &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// EventsURL returns the absolute SSE URL for a job. The SSE consumer
// uses a separate, untimed HTTP client.
func (c *Client) EventsURL(sessionID, jobID string) string {
	return fmt.Sprintf(
		"%s/api/session/%s/jobs/%s/events",
		c.baseURL,
		url.PathEscape(sessionID),
		url.PathEscape(jobID),
	)
}

// ─────────── internals ───────────

// HTTPError is returned when the backend responds with a non-2xx.
// Carries the status code so callers can distinguish 404 (session
// missing → re-create) from 401/403 (auth → re-connect service).
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("backend returned %d: %s", e.Status, e.Body)
}

func (c *Client) do(
	ctx context.Context,
	method, path string,
	body any,
	csrf string,
	out any,
) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &HTTPError{Status: resp.StatusCode, Body: string(respBody)}
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	return json.Unmarshal(respBody, out)
}
