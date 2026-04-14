package ytmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// YT Music's internal API is the same InnerTube endpoint YouTube's web
// client uses, just with a different client name ("WEB_REMIX"). Every
// request is a POST carrying a `context` object that identifies the
// client. ytmusicapi pins clientVersion to a date-stamped build of the
// real web client; we do the same.
//
// The clientVersion string is not load-bearing in any strict sense —
// YT accepts a wide range — but using a recent one reduces the chance
// of YT returning a stripped-down response or rejecting us as stale.
const (
	innertubeBase = "https://music.youtube.com/youtubei/v1"
	clientName    = "WEB_REMIX"
	clientVersion = "1.20250101.01.00" // bump occasionally; any recent value works
)

// innertubeContext is the `context` key every request body needs.
// Fields pinned to what the real YT Music web client sends; extra
// fields (hl, gl, lockedSafetyMode, useSsl) keep the response format
// consistent with what our parsers expect.
func innertubeContext() map[string]any {
	return map[string]any{
		"client": map[string]any{
			"clientName":    clientName,
			"clientVersion": clientVersion,
			"hl":            "en",
			"gl":            "US",
		},
		"user": map[string]any{
			"lockedSafetyMode": false,
		},
		"request": map[string]any{
			"useSsl": true,
		},
	}
}

// browse issues a POST /browse with the given browseId (or raw extra
// fields) and returns the decoded response body. The caller walks the
// nested JSON tree.
//
// We return `map[string]any` rather than a typed struct because the
// response shape varies wildly by browseId — typing every possible
// renderer would be more work than the walker helpers cost, and YT
// changes renderer nesting silently enough that overly-rigid types
// would break in production.
func (c *Client) browse(ctx context.Context, body map[string]any) (map[string]any, error) {
	if err := c.ensureFresh(ctx); err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	// Always set context; callers only supply browseId / continuation / etc.
	body["context"] = innertubeContext()

	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		innertubeBase+"/browse", bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.tok.Authorization())
	// YT Music's auth check is finicky about this header when using OAuth
	// tokens; omit and some library endpoints will return empty.
	req.Header.Set("X-Goog-Request-Time-Iso", time.Now().UTC().Format(time.RFC3339))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("browse: status %d: %s", resp.StatusCode, truncate(string(b), 200))
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
