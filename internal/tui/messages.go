package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musictui-go/internal/audio"
	"github.com/iamteedoh/musictui-go/internal/lyrics"
	"github.com/iamteedoh/musictui-go/internal/model"
	sp "github.com/iamteedoh/musictui-go/internal/spotify"
	"github.com/iamteedoh/musictui-go/internal/tui/components"
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

// Commands that fetch data asynchronously
func FetchLibraryCmd(client *sp.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetSavedTracks(context.Background(), offset, 10)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return LibraryLoadedMsg{Tracks: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchPlaylistsCmd(client *sp.Client, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetPlaylists(context.Background(), offset, 10)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistsLoadedMsg{Playlists: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchPlaylistTracksCmd(client *sp.Client, playlistID string, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetPlaylistTracks(context.Background(), playlistID, offset, 10)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return PlaylistTracksLoadedMsg{PlaylistID: playlistID, Tracks: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchArtistAlbumsCmd(client *sp.Client, artist model.Artist, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetArtistAlbums(context.Background(), artist.ID, offset, 10)
		if err != nil {
			return DataErrorMsg{Err: err}
		}
		return ArtistAlbumsLoadedMsg{Artist: artist, Albums: page.Items, Total: page.Total, Offset: page.Offset}
	}
}

func FetchAlbumTracksCmd(client *sp.Client, album model.Album, offset int) tea.Cmd {
	return func() tea.Msg {
		page, err := client.GetAlbumTracks(context.Background(), album.ID, offset, 10)
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
