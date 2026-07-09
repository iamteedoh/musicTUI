package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/importbackend"
	"github.com/iamteedoh/musicTUI/internal/importcore/importer"
	"github.com/iamteedoh/musicTUI/internal/lyrics"
	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
	"github.com/iamteedoh/musicTUI/internal/update"
)

// Navigation
type NavigateMsg struct{ View model.View }
type NavigateBackMsg struct{}

// Tick for animations
type TickMsg struct{}

// Status messages
type StatusMsg string
type ClearStatusMsg struct{}

// Auth messages
type AuthStartMsg struct{}
type AuthURLMsg struct{ URL string }
type AuthSuccessMsg struct {
	Client      *sp.Client
	Username    string
	AccessToken string
}
type AuthErrorMsg struct{ Err error }

// AppOwnerNotPremiumMsg signals that Spotify rejected API calls because the
// Developer app behind the configured client_id is owned by a non-Premium
// account. This is distinct from a normal auth failure — re-auth won't help;
// the user must fix the app owner's subscription or recreate the app.
type AppOwnerNotPremiumMsg struct{}

type SaveClientIDMsg struct{ ClientID string }

// Data fetching messages
type FetchLibraryMsg struct{ Offset int }
type FetchPlaylistsMsg struct{ Offset int }
type FetchPlaylistTracksMsg struct {
	PlaylistID string
	Offset     int
}
type FetchArtistAlbumsMsg struct {
	Artist model.Artist
	Offset int
}
type FetchAlbumTracksMsg struct {
	Album  model.Album
	Offset int
}
type SearchExecuteMsg struct{ Query string }

// Data loaded messages
type LibraryLoadedMsg struct {
	Tracks []model.Track
	Total  uint32
	Offset uint32
}
type PlaylistsLoadedMsg struct {
	Playlists []model.Playlist
	Total     uint32
	Offset    uint32
}
type PlaylistTracksLoadedMsg struct {
	PlaylistID string
	Tracks     []model.Track
	Total      uint32
	Offset     uint32
}
type ArtistAlbumsLoadedMsg struct {
	Artist model.Artist
	Albums []model.Album
	Total  uint32
	Offset uint32
}
type AlbumTracksLoadedMsg struct {
	Album  model.Album
	Tracks []model.Track
	Total  uint32
	Offset uint32
}
type SearchLoadedMsg struct {
	Results model.SearchResults
	Total   uint32
	Offset  uint32
	Append  bool // true if this is a pagination fetch (append to existing)
}
type DataErrorMsg struct{ Err error }

// Playlist mutation messages
type PlaylistCreatedMsg struct {
	Playlist model.Playlist
}
type PlaylistUpdatedMsg struct {
	PlaylistID string
	NewName    string
	NewDesc    string
}
type PlaylistDeletedMsg struct {
	PlaylistID string
}
type TrackAddedToPlaylistMsg struct {
	PlaylistID string
}
type TrackRemovedFromPlaylistMsg struct {
	PlaylistID string
	TrackURI   string
}
type TrackMovedMsg struct {
	FromPlaylistID string
	ToPlaylistID   string
	TrackURI       string
}
type DuplicatesConsolidatedMsg struct {
	MergedCount  int
	DeletedCount int
	BackupPath   string
}
type EmptyPlaylistsDeletedMsg struct {
	DeletedCount int
	BackupPath   string
}

// PlaylistsRestoredMsg reports the result of restoring playlists from a backup.
type PlaylistsRestoredMsg struct {
	Refollowed int
	Recreated  int
	Failed     int
}

// Commands that fetch data asynchronously
func FetchLibraryCmd(client *sp.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetSavedTracks(context.Background(), offset, 50)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return LibraryLoadedMsg{Tracks: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

// fetchPageWithRetry runs fetch up to three times with linear backoff
// (1s, 2s). The auto-pagination chains (each loaded page fetches the next)
// previously died silently on the first error — under Development Mode rate
// limits (429) that left a playlist stuck showing a partial page of tracks
// until some unrelated action happened to re-fetch (MUS-11).
func fetchPageWithRetry[T any](fetch func() (model.Page[T], error)) (model.Page[T], error) {
	var page model.Page[T]
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		page, err = fetch()
		if err == nil {
			return page, nil
		}
	}
	return page, err
}

func FetchPlaylistsCmd(client *sp.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := fetchPageWithRetry(func() (model.Page[model.Playlist], error) {
			return client.GetPlaylists(context.Background(), offset, 50)
		})
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistsLoadedMsg{Playlists: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchPlaylistTracksCmd(client *sp.Client, playlistID string, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := fetchPageWithRetry(func() (model.Page[model.Track], error) {
			return client.GetPlaylistTracks(context.Background(), playlistID, offset, 50)
		})
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistTracksLoadedMsg{PlaylistID: playlistID, Tracks: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchArtistAlbumsCmd(client *sp.Client, artist model.Artist, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetArtistAlbums(context.Background(), artist.ID, offset, 50)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return ArtistAlbumsLoadedMsg{Artist: artist, Albums: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchAlbumTracksCmd(client *sp.Client, album model.Album, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetAlbumTracks(context.Background(), album.ID, offset, 50)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return AlbumTracksLoadedMsg{Album: album, Tracks: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func SearchCmd(client *sp.Client, query string) tea.Cmd {
	return func() tea.Msg {
		results, total, err := client.Search(context.Background(), query)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return SearchLoadedMsg{Results: results, Total: total, Offset: 0}
	}
}

func SearchMoreCmd(client *sp.Client, query string, offset int) tea.Cmd {
	return func() tea.Msg {
		results, total, err := client.Search(context.Background(), query, offset)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return SearchLoadedMsg{Results: results, Total: total, Offset: uint32(offset), Append: true}
	}
}

// Lyrics messages
type LyricsLoadedMsg struct {
	Result  *lyrics.Result
	TrackID string
}

func FetchLyricsCmd(trackName, artistName string, durationSec int, trackID string) tea.Cmd {
	return func() tea.Msg {
		result, err := lyrics.Fetch(trackName, artistName, durationSec)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return LyricsLoadedMsg{Result: result, TrackID: trackID}
	}
}

// Artwork messages
type ArtworkLoadedMsg struct {
	Result components.ArtworkResult
}

func FetchArtworkCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return ArtworkLoadedMsg{Result: components.FetchArtwork(url)}
	}
}

// Self-update messages
type UpdateCheckResultMsg struct {
	Release *update.Release // nil if no update / check failed
}
type UpdateStartedMsg struct{}
type UpdateAppliedMsg struct{ NewVersion string }
type UpdateFailedMsg struct{ Err error }

func CheckForUpdateCmd(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		rel, err := update.LatestRelease(ctx)
		if err != nil || rel == nil {
			return UpdateCheckResultMsg{Release: nil}
		}
		if !update.IsNewer(currentVersion, rel.TagName) {
			return UpdateCheckResultMsg{Release: nil}
		}
		return UpdateCheckResultMsg{Release: rel}
	}
}

func ApplyUpdateCmd(rel *update.Release) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := update.DownloadAndApplyLatest(ctx, rel); err != nil {
			return UpdateFailedMsg{Err: err}
		}
		return UpdateAppliedMsg{NewVersion: rel.TagName}
	}
}

// ═════════ Import messages (v0.3.0 — embedded) ═════════

// ServicesStatusMsg reports which services have local tokens.
type ServicesStatusMsg struct {
	Status importbackend.ServicesStatus
}
type ServicesStatusErrorMsg struct{ Err error }

// ServiceAuthedMsg lands when a single service OAuth flow completed.
// Followed-up by another ServicesStatusCmd to re-check state.
type ServiceAuthedMsg struct {
	Service string // "youtube" | "spotify"
}
type ServiceAuthErrorMsg struct {
	Service string
	Err     error
}

// ImportLibraryLoadedMsg / ImportLibraryErrorMsg after calling
// Client.LoadLibrary. Named with the "Import" prefix to avoid
// collision with the Spotify-library messages further up.
type ImportLibraryLoadedMsg struct {
	Library *importbackend.YouTubeLibrary
}
type ImportLibraryErrorMsg struct{ Err error }

// ImportEventMsg carries one event from the importer channel.
// Re-arm ListenImportEventCmd until a terminal event arrives.
type ImportEventMsg struct {
	Event importer.Event
}
type ImportStreamClosedMsg struct{}

// CheckServicesCmd reads the local token store and reports which
// services have tokens. Cheap — no network.
func CheckServicesCmd(client *importbackend.Client) tea.Cmd {
	return func() tea.Msg {
		st, err := client.Services()
		if err != nil {
			return ServicesStatusErrorMsg{Err: err}
		}
		return ServicesStatusMsg{Status: st}
	}
}

// AuthServiceCmd opens the browser, runs the local OAuth loopback
// flow for `service`, and persists the resulting token. Blocks
// until the browser redirects back or ctx times out (~10 min).
func AuthServiceCmd(client *importbackend.Client, service string) tea.Cmd {
	return authServiceCmd(client, service, false)
}

// ReauthServiceCmd discards the cached token for a service, then opens
// a fresh OAuth flow. Use this when a provider reports invalid_grant or
// another token-refresh failure; retrying the stale token would loop.
func ReauthServiceCmd(client *importbackend.Client, service string) tea.Cmd {
	return authServiceCmd(client, service, true)
}

func authServiceCmd(client *importbackend.Client, service string, clearFirst bool) tea.Cmd {
	return func() tea.Msg {
		if clearFirst {
			if err := client.DeleteServiceToken(service); err != nil {
				return ServiceAuthErrorMsg{Service: service, Err: err}
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		var err error
		switch service {
		case "youtube":
			err = client.AuthYouTube(ctx)
		case "spotify":
			err = client.AuthSpotify(ctx)
		default:
			err = fmt.Errorf("unknown service %q", service)
		}
		if err != nil {
			return ServiceAuthErrorMsg{Service: service, Err: err}
		}
		return ServiceAuthedMsg{Service: service}
	}
}

// LoadLibraryCmd calls Client.LoadLibrary. Does a live YT Data API
// roundtrip; may take a few seconds for libraries with many
// playlists.
func LoadLibraryCmd(client *importbackend.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		lib, err := client.LoadLibrary(ctx)
		if err != nil {
			return ImportLibraryErrorMsg{Err: err}
		}
		return ImportLibraryLoadedMsg{Library: lib}
	}
}

// StartImportMsg carries the event channel from Client.StartImport
// back to app.go so it can be stored for ListenImportEventCmd.
type StartImportMsg struct {
	Events <-chan importer.Event
	Cancel context.CancelFunc
}

// StartImportCmd kicks the importer in a goroutine. Returns
// StartImportMsg immediately — the import itself runs to completion
// in the background, driven by ListenImportEventCmd re-arms.
func StartImportCmd(client *importbackend.Client, includeLiked bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		events := client.StartImport(ctx, importbackend.ImportRequest{
			Source:       "youtube",
			Dest:         "spotify",
			IncludeLiked: includeLiked,
		})
		return StartImportMsg{Events: events, Cancel: cancel}
	}
}

// ListenImportEventCmd reads one event from the channel. The app's
// message handler re-issues the Cmd after each event until a
// terminal event (job_done / error) arrives.
func ListenImportEventCmd(events <-chan importer.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return ImportStreamClosedMsg{}
		}
		return ImportEventMsg{Event: ev}
	}
}

// Playback messages
type PlayTrackMsg struct {
	Track model.Track
}
type PlayQueueMsg struct {
	Tracks   []model.Track
	StartIdx int
}
type AudioEventMsg struct {
	Event audio.Event
}
type TogglePlayPauseMsg struct{}

// ListenForAudioEvents returns a command that listens for engine events.
func ListenForAudioEvents(engine *audio.Engine) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-engine.Events()
		if !ok {
			return AudioEventMsg{Event: audio.Event{Kind: "stopped"}}
		}
		return AudioEventMsg{Event: ev}
	}
}

// Playlist mutation commands
func CreatePlaylistCmd(client *sp.Client, name, description string) tea.Cmd {
	return func() tea.Msg {
		pl, err := client.CreatePlaylist(context.Background(), name, description, false)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistCreatedMsg{Playlist: pl}
	}
}

func UpdatePlaylistCmd(client *sp.Client, playlistID, name, description string) tea.Cmd {
	return func() tea.Msg {
		if err := client.UpdatePlaylistDetails(context.Background(), playlistID, name, description); err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistUpdatedMsg{PlaylistID: playlistID, NewName: name, NewDesc: description}
	}
}

func DeletePlaylistCmd(client *sp.Client, playlistID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.DeletePlaylist(context.Background(), playlistID); err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistDeletedMsg{PlaylistID: playlistID}
	}
}

func AddTrackToPlaylistCmd(client *sp.Client, playlistID, trackURI string) tea.Cmd {
	return func() tea.Msg {
		if err := client.AddTracksToPlaylist(context.Background(), playlistID, []string{trackURI}); err != nil {
			return DataErrorMsg{Err: err}
		}
		return TrackAddedToPlaylistMsg{PlaylistID: playlistID}
	}
}

func RemoveTrackFromPlaylistCmd(client *sp.Client, playlistID, trackURI string) tea.Cmd {
	return func() tea.Msg {
		if err := client.RemoveTracksFromPlaylist(context.Background(), playlistID, []string{trackURI}); err != nil {
			return DataErrorMsg{Err: err}
		}
		return TrackRemovedFromPlaylistMsg{PlaylistID: playlistID, TrackURI: trackURI}
	}
}

func MoveTrackCmd(client *sp.Client, fromPlaylistID, toPlaylistID, trackURI string) tea.Cmd {
	return func() tea.Msg {
		if err := client.AddTracksToPlaylist(context.Background(), toPlaylistID, []string{trackURI}); err != nil {
			return DataErrorMsg{Err: err}
		}
		if err := client.RemoveTracksFromPlaylist(context.Background(), fromPlaylistID, []string{trackURI}); err != nil {
			return DataErrorMsg{Err: err}
		}
		return TrackMovedMsg{FromPlaylistID: fromPlaylistID, ToPlaylistID: toPlaylistID, TrackURI: trackURI}
	}
}

// ConsolidateDuplicatesCmd merges playlists with identical names.
// For each group of duplicates: fetches all tracks, deduplicates by track ID
// (different versions/lengths of a song have different IDs), adds unique tracks
// to the first playlist, and deletes the rest.
func ConsolidateDuplicatesCmd(client *sp.Client, groups [][]model.Playlist) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var mergedCount, deletedCount int

		// Snapshot every playlist that is about to be unfollowed so it can be
		// restored in-app (press R in the Playlists view) if this was a
		// mistake. Failure to back up is non-fatal but reported via the path.
		var toRemove []model.Playlist
		for _, group := range groups {
			if len(group) >= 2 {
				toRemove = append(toRemove, group[1:]...)
			}
		}
		backupPath, _ := client.SnapshotPlaylists(ctx, toRemove, "consolidate-duplicates")

		for _, group := range groups {
			if len(group) < 2 {
				continue
			}
			keep := group[0]
			duplicates := group[1:]

			seen := make(map[string]bool)

			// Fetch tracks from the keeper
			offset := 0
			for {
				page, err := client.GetPlaylistTracks(ctx, keep.ID, offset, 50)
				if err != nil {
					break
				}
				for _, t := range page.Items {
					seen[t.ID] = true
				}
				if uint32(offset+len(page.Items)) >= page.Total || len(page.Items) == 0 {
					break
				}
				offset += len(page.Items)
			}

			// Fetch tracks from duplicates
			var newURIs []string
			for _, dup := range duplicates {
				offset = 0
				for {
					page, err := client.GetPlaylistTracks(ctx, dup.ID, offset, 50)
					if err != nil {
						break
					}
					for _, t := range page.Items {
						if !seen[t.ID] {
							seen[t.ID] = true
							newURIs = append(newURIs, t.URI)
						}
					}
					if uint32(offset+len(page.Items)) >= page.Total || len(page.Items) == 0 {
						break
					}
					offset += len(page.Items)
				}
			}

			// Add unique tracks to the keeper
			for i := 0; i < len(newURIs); i += 100 {
				end := i + 100
				if end > len(newURIs) {
					end = len(newURIs)
				}
				if err := client.AddTracksToPlaylist(ctx, keep.ID, newURIs[i:end]); err != nil {
					return DataErrorMsg{Err: fmt.Errorf("adding tracks to %s: %w", keep.Name, err)}
				}
			}

			// Delete the duplicate playlists
			for _, dup := range duplicates {
				if err := client.DeletePlaylist(ctx, dup.ID); err != nil {
					return DataErrorMsg{Err: fmt.Errorf("deleting duplicate %s: %w", dup.Name, err)}
				}
				deletedCount++
			}
			mergedCount++
		}

		return DuplicatesConsolidatedMsg{MergedCount: mergedCount, DeletedCount: deletedCount, BackupPath: backupPath}
	}
}

func DeleteEmptyPlaylistsCmd(client *sp.Client, playlists []model.Playlist) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		// Back up before unfollowing so the action is recoverable in-app.
		backupPath, _ := client.SnapshotPlaylists(ctx, playlists, "delete-empty-playlists")
		var count int
		for _, pl := range playlists {
			if err := client.DeletePlaylist(ctx, pl.ID); err != nil {
				return DataErrorMsg{Err: fmt.Errorf("deleting %s: %w", pl.Name, err)}
			}
			count++
		}
		return EmptyPlaylistsDeletedMsg{DeletedCount: count, BackupPath: backupPath}
	}
}

// RestorePlaylistsCmd restores playlists from the most recent backup written
// before a merge/delete. Each playlist is re-followed by its original ID, or
// recreated from the snapshot if it no longer exists on Spotify.
func RestorePlaylistsCmd(client *sp.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		bf, _, err := sp.LoadLatestBackup()
		if err != nil {
			return DataErrorMsg{Err: fmt.Errorf("loading backup: %w", err)}
		}
		refollowed, recreated, failed := client.RestoreFromBackup(ctx, bf)
		return PlaylistsRestoredMsg{Refollowed: refollowed, Recreated: recreated, Failed: failed}
	}
}

// SixelEncodedMsg carries a cover encoded off the event loop. Encoding costs
// tens of milliseconds; doing it in View froze the app for a frame and a resize
// drag jammed it solid (MUS-29).
type SixelEncodedMsg struct {
	URL        string
	Cols, Rows int
	Payload    string
}

// EncodeSixelCmd encodes the pending cover in a command goroutine.
func EncodeSixelCmd(w components.SixelWork) tea.Cmd {
	return func() tea.Msg {
		payload, err := components.EncodeSixel(w)
		if err != nil {
			return SixelEncodedMsg{URL: w.URL, Cols: w.Cols, Rows: w.Rows}
		}
		return SixelEncodedMsg{URL: w.URL, Cols: w.Cols, Rows: w.Rows, Payload: payload}
	}
}

// sixelRepaintMsg asks for the cover to be painted again. It is sequenced after
// tea.ClearScreen on resize: the erase has to reach the terminal before the
// image, or the cover is painted and immediately wiped (MUS-29).
type sixelRepaintMsg struct{}

func sixelRepaintCmd() tea.Cmd {
	return func() tea.Msg { return sixelRepaintMsg{} }
}
