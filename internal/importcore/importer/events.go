// Package importer orchestrates a library transfer: read YT
// playlists + tracks, filter non-music, score-match each track
// against Spotify search results, create new Spotify playlists with
// the matches. Emits progress events on a Go channel so both the
// TUI (BubbleTea tea.Cmd loop) and the standalone CLI (progress bar)
// can consume them.
package importer

// EventType enumerates the categories of progress event the Run()
// goroutine emits. Matches the SSE taxonomy the hosted backend used
// in v0.2.0 so client code stays familiar.
type EventType string

const (
	EventJobStarted      EventType = "job_started"
	EventPlaylistStarted EventType = "playlist_started"
	EventPlaylistSkipped EventType = "playlist_skipped"
	EventTrackMatched    EventType = "track_matched"
	EventTrackUnmatched  EventType = "track_unmatched"
	EventPlaylistDone    EventType = "playlist_done"
	EventJobDone         EventType = "job_done"
	EventError           EventType = "error"
)

// Event is one progress datum. Fields are optional; consumers read
// what's populated based on Type.
type Event struct {
	Type EventType

	// Playlist-level fields
	PlaylistName     string
	PlaylistURL      string // spotify open.spotify.com URL after create
	PlaylistTotal    int    // #tracks in this playlist
	FilteredNonMusic int    // #tracks dropped by category filter

	// Track-level fields
	TrackIndex      int // 1-based position within current playlist
	TrackTitle      string
	TrackArtist     string
	TrackConfidence float64
	TrackURI        string // matched Spotify URI, empty on unmatched
	TrackReason     string // "below threshold" / "no candidates" / "search failed: ..."

	// Job-level fields
	Source        string
	Dest          string
	Matched       int
	Unmatched     int
	Errors        int
	PlaylistCount int

	// Skip reason
	SkipReason string // "no music tracks (all filtered by category)" / "already exists in Spotify"

	// Error fallback
	Message string
}
