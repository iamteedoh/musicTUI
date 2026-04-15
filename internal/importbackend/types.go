// Package importbackend is the CLI's thin client for the
// musictui-import backend (FastAPI service at
// musictui-import.iamteedoh.dev or self-hosted).
//
// The whole library import flow used to live in this binary
// (internal/ytmusic + internal/applemusic). It moved to a server in
// v0.2.0 because the YT Music internal API rejects OAuth tokens minted
// by public client apps — the only way to read a YT Music library at
// scale is with a properly-registered Google OAuth client, which the
// backend holds.
package importbackend

// Session is the per-CLI handle the backend issues. We persist it at
// ~/.config/musicTUI/import-session.json so the import flow on second
// launch is a no-op (skip straight to library read).
type Session struct {
	SessionID string `json:"session_id"`
	CSRFToken string `json:"csrf_token"`
	ExpiresAt string `json:"expires_at"`
	BackendURL string `json:"backend_url"`
}

// PlaylistSummary mirrors the backend's response shape for
// /api/session/{id}/library/youtube.
type PlaylistSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
}

type YouTubeLibrary struct {
	Playlists  []PlaylistSummary `json:"playlists"`
	LikedCount int               `json:"liked_count"`
}

// SessionState is the GET /api/session/{id} response — used to poll
// for OAuth completion (services map flips from false → true).
type SessionState struct {
	SessionID string          `json:"session_id"`
	ExpiresAt string          `json:"expires_at"`
	Services  map[string]bool `json:"services"`
}

// ImportRequest is the POST /api/session/{id}/import body.
type ImportRequest struct {
	Source       string   `json:"source"`
	Dest         string   `json:"dest"`
	PlaylistIDs  []string `json:"playlist_ids,omitempty"`
	IncludeLiked bool     `json:"include_liked"`
}

type StartImportResponse struct {
	JobID string `json:"job_id"`
}

// JobState mirrors GET /api/session/{id}/jobs/{job_id} — the polling
// fallback when SSE isn't usable. Summary is a free-form blob the
// backend assembles when the job lands in "done".
type JobState struct {
	JobID           string         `json:"job_id"`
	Status          string         `json:"status"`
	SourceService   string         `json:"source_service"`
	DestService     string         `json:"dest_service"`
	ProgressCurrent int            `json:"progress_current"`
	ProgressTotal   int            `json:"progress_total"`
	Error           string         `json:"error"`
	Summary         map[string]any `json:"summary"`
}

// ─────────── SSE event payloads ───────────

// Event is one decoded SSE frame. Type maps to the backend's event
// taxonomy: job_started, playlist_started, track_matched,
// track_unmatched, playlist_done, job_done, error.
type Event struct {
	Type string         `json:"-"`
	Seq  int            `json:"-"`
	Data map[string]any `json:"-"`
}

// Convenience accessors — saves the TUI from `.(string)` boilerplate.

func (e Event) Str(key string) string {
	if v, ok := e.Data[key].(string); ok {
		return v
	}
	return ""
}

func (e Event) Int(key string) int {
	switch v := e.Data[key].(type) {
	case float64: // JSON numbers decode to float64
		return int(v)
	case int:
		return v
	}
	return 0
}

func (e Event) Float(key string) float64 {
	if v, ok := e.Data[key].(float64); ok {
		return v
	}
	return 0
}
