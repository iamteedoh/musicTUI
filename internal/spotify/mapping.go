package spotify

import (
	"time"

	spotifylib "github.com/zmb3/spotify/v2"

	"github.com/iamteedoh/musictui-go/internal/model"
)

// ── Raw API types for the Feb 2026 Spotify response format ──

// rawPlaylistPage is the response from GET /me/playlists.
type rawPlaylistPage struct {
	Items  []rawPlaylist `json:"items"`
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
}

// rawPlaylist is a playlist object from the new API format.
// The old "tracks" field was renamed to "items".
type rawPlaylist struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	URI         string         `json:"uri"`
	Owner       rawOwner       `json:"owner"`
	Images      []rawImage     `json:"images"`
	Items       *rawItemsCount `json:"items"` // was "tracks" pre-2026
}

type rawOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type rawImage struct {
	URL string `json:"url"`
}

// rawItemsCount is the track/item count in the playlist listing.
type rawItemsCount struct {
	Href  string `json:"href"`
	Total int    `json:"total"`
}

// rawPlaylistItemsPage is the response from GET /playlists/{id}/items.
type rawPlaylistItemsPage struct {
	Items  []rawPlaylistItem `json:"items"`
	Total  int               `json:"total"`
	Offset int               `json:"offset"`
	Limit  int               `json:"limit"`
}

type rawPlaylistItem struct {
	AddedAt string    `json:"added_at"`
	Item    *rawTrack `json:"item"` // was "track" pre-2026
}

type rawTrack struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	URI         string           `json:"uri"`
	DurationMs  int             `json:"duration_ms"`
	TrackNumber int             `json:"track_number"`
	DiscNumber  int             `json:"disc_number"`
	Explicit    bool            `json:"explicit"`
	Artists     []rawArtistRef  `json:"artists"`
	Album       *rawAlbumRef    `json:"album"`
}

type rawArtistRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type rawAlbumRef struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Images []rawImage `json:"images"`
}

func mapArtist(a spotifylib.SimpleArtist) model.Artist {
	return model.Artist{
		ID:   string(a.ID),
		Name: a.Name,
		URI:  string(a.URI),
	}
}

func mapFullArtist(a spotifylib.FullArtist) model.Artist {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	return model.Artist{
		ID:       string(a.ID),
		Name:     a.Name,
		ImageURL: img,
		URI:      string(a.URI),
	}
}

func mapAlbumRef(a spotifylib.SimpleAlbum) *model.AlbumRef {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	return &model.AlbumRef{
		ID:       string(a.ID),
		Name:     a.Name,
		ImageURL: img,
	}
}

func mapAlbum(a spotifylib.SimpleAlbum) model.Album {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	artists := make([]model.Artist, len(a.Artists))
	for i, art := range a.Artists {
		artists[i] = mapArtist(art)
	}
	return model.Album{
		ID:          string(a.ID),
		Name:        a.Name,
		Artists:     artists,
		ReleaseDate: a.ReleaseDate,
		ImageURL:    img,
		URI:         string(a.URI),
	}
}

func mapTrack(t spotifylib.FullTrack) model.Track {
	artists := make([]model.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = mapArtist(a)
	}
	var album *model.AlbumRef
	if t.Album.ID != "" {
		album = mapAlbumRef(t.Album)
	}
	return model.Track{
		ID:          string(t.ID),
		Name:        t.Name,
		Artists:     artists,
		Album:       album,
		Duration:    time.Duration(t.Duration) * time.Millisecond,
		TrackNumber: uint32(t.TrackNumber),
		DiscNumber:  uint32(t.DiscNumber),
		Explicit:    t.Explicit,
		URI:         string(t.URI),
	}
}

func mapSimpleTrack(t spotifylib.SimpleTrack) model.Track {
	artists := make([]model.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = mapArtist(a)
	}
	return model.Track{
		ID:          string(t.ID),
		Name:        t.Name,
		Artists:     artists,
		Duration:    time.Duration(t.Duration) * time.Millisecond,
		TrackNumber: uint32(t.TrackNumber),
		DiscNumber:  uint32(t.DiscNumber),
		Explicit:    t.Explicit,
		URI:         string(t.URI),
	}
}

// mapPlaylist maps a zmb3 SimplePlaylist (used by search results).
func mapPlaylist(p spotifylib.SimplePlaylist) model.Playlist {
	img := ""
	if len(p.Images) > 0 {
		img = p.Images[0].URL
	}
	owner := ""
	if p.Owner.DisplayName != "" {
		owner = p.Owner.DisplayName
	} else {
		owner = string(p.Owner.ID)
	}
	return model.Playlist{
		ID:       string(p.ID),
		Name:     p.Name,
		Owner:    owner,
		ImageURL: img,
		URI:      string(p.URI),
	}
}

// mapRawPlaylist maps the new Spotify API playlist format.
func mapRawPlaylist(p rawPlaylist) model.Playlist {
	img := ""
	if len(p.Images) > 0 {
		img = p.Images[0].URL
	}
	owner := p.Owner.DisplayName
	if owner == "" {
		owner = p.Owner.ID
	}
	var trackCount uint32
	if p.Items != nil {
		trackCount = uint32(p.Items.Total)
	}
	return model.Playlist{
		ID:         p.ID,
		Name:       p.Name,
		Owner:      owner,
		TrackCount: trackCount,
		ImageURL:   img,
		URI:        p.URI,
	}
}

// mapRawTrack maps a track from the new Spotify API format.
func mapRawTrack(t rawTrack) model.Track {
	artists := make([]model.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = model.Artist{ID: a.ID, Name: a.Name, URI: a.URI}
	}
	var album *model.AlbumRef
	if t.Album != nil && t.Album.ID != "" {
		img := ""
		if len(t.Album.Images) > 0 {
			img = t.Album.Images[0].URL
		}
		album = &model.AlbumRef{ID: t.Album.ID, Name: t.Album.Name, ImageURL: img}
	}
	return model.Track{
		ID:          t.ID,
		Name:        t.Name,
		Artists:     artists,
		Album:       album,
		Duration:    time.Duration(t.DurationMs) * time.Millisecond,
		TrackNumber: uint32(t.TrackNumber),
		DiscNumber:  uint32(t.DiscNumber),
		Explicit:    t.Explicit,
		URI:         t.URI,
	}
}
