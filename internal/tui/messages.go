package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/audio"
	"github.com/iamteedoh/musicTUI/internal/lyrics"
	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
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
