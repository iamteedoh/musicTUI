package spotify

import (
	"context"
	"sort"
	"strings"
	"sync"

	spotifylib "github.com/zmb3/spotify/v2"

	"github.com/iamteedoh/musictui-go/internal/model"
)

const devModeLimit = 10

// Client wraps the zmb3/spotify client with our domain models.
type Client struct {
	sp       *spotifylib.Client
	username string
	mu       sync.RWMutex
}

// NewClient creates an authenticated Spotify client.
func NewClient(sp *spotifylib.Client) *Client {
	return &Client{sp: sp}
}

// FetchUsername fetches and caches the current user's display name.
func (c *Client) FetchUsername(ctx context.Context) (string, error) {
	user, err := c.sp.CurrentUser(ctx)
	if err != nil {
		return "", err
	}
	name := user.DisplayName
	if name == "" {
		name = string(user.ID)
	}
	c.mu.Lock()
	c.username = name
	c.mu.Unlock()
	return name, nil
}

func (c *Client) Username() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.username
}

// GetSavedTracks returns the user's liked tracks.
func (c *Client) GetSavedTracks(ctx context.Context, offset, limit int) (model.Page[model.Track], error) {
	if limit > devModeLimit {
		limit = devModeLimit
	}
	opts := []spotifylib.RequestOption{
		spotifylib.Limit(limit),
		spotifylib.Offset(offset),
	}
	page, err := c.sp.CurrentUsersTracks(ctx, opts...)
	if err != nil {
		return model.Page[model.Track]{}, err
	}
	tracks := make([]model.Track, 0, len(page.Tracks))
	for _, st := range page.Tracks {
		tracks = append(tracks, mapTrack(st.FullTrack))
	}
	return model.Page[model.Track]{
		Items:  tracks,
		Total:  uint32(page.Total),
		Offset: uint32(page.Offset),
		Limit:  uint32(page.Limit),
	}, nil
}

// GetPlaylists returns the current user's playlists.
func (c *Client) GetPlaylists(ctx context.Context, offset, limit int) (model.Page[model.Playlist], error) {
	if limit > devModeLimit {
		limit = devModeLimit
	}
	opts := []spotifylib.RequestOption{
		spotifylib.Limit(limit),
		spotifylib.Offset(offset),
	}
	page, err := c.sp.CurrentUsersPlaylists(ctx, opts...)
	if err != nil {
		return model.Page[model.Playlist]{}, err
	}
	playlists := make([]model.Playlist, 0, len(page.Playlists))
	for _, p := range page.Playlists {
		playlists = append(playlists, mapPlaylist(p))
	}
	return model.Page[model.Playlist]{
		Items:  playlists,
		Total:  uint32(page.Total),
		Offset: uint32(page.Offset),
		Limit:  uint32(page.Limit),
	}, nil
}

// GetPlaylistTracks returns tracks in a specific playlist.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string, offset, limit int) (model.Page[model.Track], error) {
	if limit > devModeLimit {
		limit = devModeLimit
	}
	opts := []spotifylib.RequestOption{
		spotifylib.Limit(limit),
		spotifylib.Offset(offset),
	}
	page, err := c.sp.GetPlaylistItems(ctx, spotifylib.ID(playlistID), opts...)
	if err != nil {
		return model.Page[model.Track]{}, err
	}
	tracks := make([]model.Track, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Track.Track != nil {
			tracks = append(tracks, mapTrack(*item.Track.Track))
		}
	}
	return model.Page[model.Track]{
		Items:  tracks,
		Total:  uint32(page.Total),
		Offset: uint32(page.Offset),
		Limit:  uint32(page.Limit),
	}, nil
}

// GetArtistAlbums returns albums by an artist.
func (c *Client) GetArtistAlbums(ctx context.Context, artistID string, offset, limit int) (model.Page[model.Album], error) {
	if limit > devModeLimit {
		limit = devModeLimit
	}
	page, err := c.sp.GetArtistAlbums(ctx, spotifylib.ID(artistID),
		[]spotifylib.AlbumType{spotifylib.AlbumTypeAlbum, spotifylib.AlbumTypeSingle, spotifylib.AlbumTypeCompilation},
		spotifylib.Limit(limit), spotifylib.Offset(offset),
	)
	if err != nil {
		return model.Page[model.Album]{}, err
	}
	albums := make([]model.Album, 0, len(page.Albums))
	for _, a := range page.Albums {
		albums = append(albums, mapAlbum(a))
	}

	// Sort: Newest album first
	sort.Slice(albums, func(i, j int) bool {
		return albums[i].ReleaseDate > albums[j].ReleaseDate
	})

	return model.Page[model.Album]{
		Items:  albums,
		Total:  uint32(page.Total),
		Offset: uint32(page.Offset),
		Limit:  uint32(page.Limit),
	}, nil
}

// GetAlbumTracks returns tracks in an album.
func (c *Client) GetAlbumTracks(ctx context.Context, albumID string, offset, limit int) (model.Page[model.Track], error) {
	if limit > devModeLimit {
		limit = devModeLimit
	}
	page, err := c.sp.GetAlbumTracks(ctx, spotifylib.ID(albumID),
		spotifylib.Limit(limit), spotifylib.Offset(offset),
	)
	if err != nil {
		return model.Page[model.Track]{}, err
	}
	tracks := make([]model.Track, 0, len(page.Tracks))
	for _, t := range page.Tracks {
		tracks = append(tracks, mapSimpleTrack(t))
	}
	return model.Page[model.Track]{
		Items:  tracks,
		Total:  uint32(page.Total),
		Offset: uint32(page.Offset),
		Limit:  uint32(page.Limit),
	}, nil
}

// Search searches Spotify across all types with pagination support.
func (c *Client) Search(ctx context.Context, query string, offset ...int) (model.SearchResults, uint32, error) {
	results := model.SearchResults{PrimaryType: "track"}
	off := 0
	if len(offset) > 0 {
		off = offset[0]
	}

	sr, err := c.sp.Search(ctx, query,
		spotifylib.SearchTypeTrack|spotifylib.SearchTypeAlbum|spotifylib.SearchTypeArtist|spotifylib.SearchTypePlaylist,
		spotifylib.Limit(devModeLimit), spotifylib.Offset(off),
	)
	if err != nil {
		return results, 0, err
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWords := strings.Fields(queryLower)

	var total uint32
	if sr.Tracks != nil {
		for _, t := range sr.Tracks.Tracks {
			results.Tracks = append(results.Tracks, mapTrack(t))
		}
		total = uint32(sr.Tracks.Total)
	}

	if sr.Albums != nil {
		for _, a := range sr.Albums.Albums {
			results.Albums = append(results.Albums, mapAlbum(a))
		}
		// Sort search result albums: Newest first
		sort.Slice(results.Albums, func(i, j int) bool {
			return results.Albums[i].ReleaseDate > results.Albums[j].ReleaseDate
		})
	}

	if sr.Artists != nil {
		// Separate exact matches from partial matches
		var exact, partial []model.Artist
		for _, a := range sr.Artists.Artists {
			nameLower := strings.ToLower(a.Name)
			mapped := mapFullArtist(a)

			if nameLower == queryLower {
				exact = append(exact, mapped)
				continue
			}

			// Check if name contains at least one query word
			match := false
			for _, word := range queryWords {
				if strings.Contains(nameLower, word) {
					match = true
					break
				}
			}
			if match {
				partial = append(partial, mapped)
			}
		}

		// If exact match exists, only show that. Otherwise show partial matches.
		if len(exact) > 0 {
			results.Artists = exact
			results.PrimaryType = "artist"
		} else {
			results.Artists = partial
		}
	}

	// Detect if query is likely an album search
	if results.PrimaryType == "track" && len(results.Albums) > 0 {
		for _, a := range results.Albums {
			if strings.ToLower(a.Name) == queryLower {
				results.PrimaryType = "album"
				break
			}
		}
	}

	if sr.Playlists != nil {
		for _, p := range sr.Playlists.Playlists {
			results.Playlists = append(results.Playlists, mapPlaylist(p))
		}
	}

	return results, total, nil
}
