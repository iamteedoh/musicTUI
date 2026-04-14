package tui

import (
	"context"
	"fmt"
	"net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/lyrics"
	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
	"github.com/iamteedoh/musicTUI/internal/update"
	"github.com/iamteedoh/musicTUI/internal/applemusic"
	"github.com/iamteedoh/musicTUI/internal/ytmusic"
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
}
type EmptyPlaylistsDeletedMsg struct {
	DeletedCount int
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

func FetchPlaylistsCmd(client *sp.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetPlaylists(context.Background(), offset, 50)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistsLoadedMsg{Playlists: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchPlaylistTracksCmd(client *sp.Client, playlistID string, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetPlaylistTracks(context.Background(), playlistID, offset, 50)
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

// ═════════ YT Music import messages ═════════

// Auth phase: device flow.
type YTDeviceCodeMsg struct {
	Auth *ytmusic.DeviceAuth
}
type YTAuthSuccessMsg struct {
	Token *ytmusic.Token
}
type YTAuthErrorMsg struct {
	Err error
}

// Library read phase.
type YTLibraryLoadedMsg struct {
	Playlists  []ytmusic.Playlist
	LikedCount int
	Albums     []ytmusic.Album
	Artists    []ytmusic.Artist
}
type YTLibraryErrorMsg struct{ Err error }

// Import phase.
type YTImportProgressMsg struct {
	PlaylistName   string
	Done, Total    int
	Overall, Count int
}
type YTImportDoneMsg struct {
	Summaries []ytmusic.ImportSummary
}
type YTImportErrorMsg struct{ Err error }

// ─────── commands ───────

// StartYTDeviceAuthCmd kicks off the device flow and returns the
// device code to display to the user. Caller should then run
// PollYTAuthCmd to watch for approval.
func StartYTDeviceAuthCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		auth, err := ytmusic.RequestDeviceCode(ctx)
		if err != nil {
			return YTAuthErrorMsg{Err: err}
		}
		return YTDeviceCodeMsg{Auth: auth}
	}
}

// PollYTAuthCmd blocks (with the caller-supplied context, which
// should be cancelled if the user navigates away) until the user
// approves the device code or it expires. Emits YTAuthSuccessMsg
// or YTAuthErrorMsg.
func PollYTAuthCmd(deviceCode string, interval int) tea.Cmd {
	return func() tea.Msg {
		// Use a generous overall timeout — device codes live ~15 minutes.
		ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
		defer cancel()
		tok, err := ytmusic.PollForToken(ctx, deviceCode, interval)
		if err != nil {
			return YTAuthErrorMsg{Err: err}
		}
		_ = ytmusic.SaveToken(tok)
		return YTAuthSuccessMsg{Token: tok}
	}
}

// LoadYTLibraryCmd fetches playlists / liked / albums / artists in
// parallel-ish sequence and returns a single message with all of
// them. The liked-songs count comes from the "LM" playlist's size.
func LoadYTLibraryCmd(tok *ytmusic.Token) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		client := ytmusic.NewClient(tok)

		playlists, err := client.GetLibraryPlaylists(ctx)
		if err != nil {
			return YTLibraryErrorMsg{Err: fmt.Errorf("playlists: %w", err)}
		}
		// Liked songs can be large; getting the count just means
		// fetching the LM playlist and returning its length. Not ideal
		// (downloads all tracks), but YT doesn't give us a header-only
		// way. Acceptable for a summary; we re-fetch during import.
		liked, err := client.GetLikedSongs(ctx)
		if err != nil {
			// Non-fatal — some users have no liked songs, and if YT is
			// being weird we'd rather show a 0 than fail the whole load.
			liked = nil
		}
		albums, err := client.GetLibraryAlbums(ctx)
		if err != nil {
			albums = nil
		}
		artists, err := client.GetLibraryArtists(ctx)
		if err != nil {
			artists = nil
		}
		return YTLibraryLoadedMsg{
			Playlists:  playlists,
			LikedCount: len(liked),
			Albums:     albums,
			Artists:    artists,
		}
	}
}

// RunYTImportCmd runs ImportPlaylists + ImportLikedSongs end-to-end
// and returns the aggregated summaries. Progress is NOT streamed here
// in the first release — the view shows a simple spinner with the
// most-recent playlist name (updated by a goroutine that writes to
// app state via an atomic pointer). We'll add real progress streaming
// if users ask for it; for a first cut, "it's doing the thing, be
// patient" is enough given the whole import typically finishes in
// under a minute for a few hundred tracks.
func RunYTImportCmd(tok *ytmusic.Token, spClient *sp.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		yt := ytmusic.NewClient(tok)

		summaries, err := ytmusic.ImportPlaylists(ctx, yt, spClient, nil)
		if err != nil {
			return YTImportErrorMsg{Err: err}
		}
		// Liked songs as an additional playlist.
		if liked, lerr := ytmusic.ImportLikedSongs(ctx, yt, spClient, nil); lerr == nil && liked != nil {
			summaries = append(summaries, *liked)
		}
		return YTImportDoneMsg{Summaries: summaries}
	}
}

// ═════════ Apple Music import messages ═════════

// AMAuthPageMsg is emitted right after we've started the local
// callback listener and constructed the browser URL. The TUI then
// opens the URL, stays on a waiting screen, and a subsequent
// AMAuthSuccessMsg lands when the user completes sign-in.
type AMAuthPageMsg struct {
	URL string // URL we opened in the browser
}
type AMAuthSuccessMsg struct {
	Creds *applemusic.Credentials
}
type AMAuthErrorMsg struct{ Err error }

type AMLibraryLoadedMsg struct {
	Playlists  []applemusic.Playlist
	LikedCount int
	Albums     []applemusic.Album
	Artists    []applemusic.Artist
}
type AMLibraryErrorMsg struct{ Err error }

type AMImportDoneMsg struct {
	Summaries []applemusic.ImportSummary
}
type AMImportErrorMsg struct{ Err error }

// StartAMAuthCmd launches the Apple Music callback-listener + opens
// the hosted auth page in the user's browser. The browser flow
// completes asynchronously; AMAuthSuccessMsg lands when MusicKit hands
// us the Music User Token.
func StartAMAuthCmd(authPageURL, developerToken string, port int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		// intentionally don't cancel() here — the callback listener
		// lives for the duration of the flow. The timeout still
		// bounds it.
		_ = cancel

		cbURL, state, ch, err := applemusic.StartCallbackServer(ctx, port)
		if err != nil {
			return AMAuthErrorMsg{Err: fmt.Errorf("start callback server: %w", err)}
		}

		// Build the browser URL with dev token + callback + state.
		u, err := url.Parse(authPageURL)
		if err != nil {
			return AMAuthErrorMsg{Err: fmt.Errorf("invalid auth_page_url: %w", err)}
		}
		q := u.Query()
		q.Set("dev", developerToken)
		q.Set("cb", cbURL)
		q.Set("state", state)
		u.RawQuery = q.Encode()

		go func() {
			// Fire in a goroutine so we can return the URL to the UI
			// immediately; the actual browser exec can take a beat.
			openBrowser(u.String())
		}()

		select {
		case res := <-ch:
			if res.Err != nil {
				return AMAuthErrorMsg{Err: res.Err}
			}
			creds := &applemusic.Credentials{
				DeveloperToken: developerToken,
				MusicUserToken: res.MusicUserToken,
				ObtainedAt:     time.Now(),
			}
			_ = applemusic.SaveCredentials(creds)
			return AMAuthSuccessMsg{Creds: creds}
		case <-ctx.Done():
			return AMAuthErrorMsg{Err: fmt.Errorf("timeout waiting for Apple Music sign-in")}
		}
	}
}

// LoadAMLibraryCmd fetches playlists / library songs / albums /
// artists. Mirrors LoadYTLibraryCmd.
func LoadAMLibraryCmd(creds *applemusic.Credentials) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		am := applemusic.NewClient(creds)
		playlists, err := am.GetLibraryPlaylists(ctx)
		if err != nil {
			return AMLibraryErrorMsg{Err: fmt.Errorf("playlists: %w", err)}
		}
		liked, _ := am.GetLikedSongs(ctx)
		albums, _ := am.GetLibraryAlbums(ctx)
		artists, _ := am.GetLibraryArtists(ctx)
		return AMLibraryLoadedMsg{
			Playlists:  playlists,
			LikedCount: len(liked),
			Albums:     albums,
			Artists:    artists,
		}
	}
}

// RunAMImportCmd does the full Apple Music import.
func RunAMImportCmd(creds *applemusic.Credentials, spClient *sp.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		am := applemusic.NewClient(creds)
		summaries, err := applemusic.ImportPlaylists(ctx, am, spClient)
		if err != nil {
			return AMImportErrorMsg{Err: err}
		}
		if liked, lerr := applemusic.ImportLikedSongs(ctx, am, spClient); lerr == nil && liked != nil {
			summaries = append(summaries, *liked)
		}
		return AMImportDoneMsg{Summaries: summaries}
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

		return DuplicatesConsolidatedMsg{MergedCount: mergedCount, DeletedCount: deletedCount}
	}
}

func DeleteEmptyPlaylistsCmd(client *sp.Client, playlists []model.Playlist) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var count int
		for _, pl := range playlists {
			if err := client.DeletePlaylist(ctx, pl.ID); err != nil {
				return DataErrorMsg{Err: fmt.Errorf("deleting %s: %w", pl.Name, err)}
			}
			count++
		}
		return EmptyPlaylistsDeletedMsg{DeletedCount: count}
	}
}
