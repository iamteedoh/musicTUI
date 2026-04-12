package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	spotifylib "github.com/zmb3/spotify/v2"

	"github.com/iamteedoh/musictui-go/internal/model"
)

const (
	maxPageSize    = 50
	spotifyBaseURL = "https://api.spotify.com/v1"
)

// Client wraps the zmb3/spotify client with our domain models.
type Client struct {
	sp         *spotifylib.Client
	httpClient *http.Client // raw HTTP client for endpoints the library doesn't handle
	username   string
	userID     string
	mu         sync.RWMutex
}

// NewClient creates an authenticated Spotify client.
func NewClient(sp *spotifylib.Client, httpClient *http.Client) *Client {
	return &Client{sp: sp, httpClient: httpClient}
}

// apiGet makes an authenticated GET request to the Spotify API.
func (c *Client) apiGet(ctx context.Context, url string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

// apiPost makes an authenticated POST request with a JSON body.
func (c *Client) apiPost(ctx context.Context, url string, body any, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, b)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// apiPut makes an authenticated PUT request with a JSON body.
func (c *Client) apiPut(ctx context.Context, url string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, b)
	}
	return nil
}

// apiDeleteWithBody makes an authenticated DELETE request with an optional JSON body.
func (c *Client) apiDeleteWithBody(ctx context.Context, url string, body any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, reqBody)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, b)
	}
	return nil
}

// CreatePlaylist creates a new playlist for the current user.
func (c *Client) CreatePlaylist(ctx context.Context, name, description string, public bool) (model.Playlist, error) {
	uid := c.UserID()
	if uid == "" {
		return model.Playlist{}, fmt.Errorf("user ID not available")
	}
	url := fmt.Sprintf("%s/users/%s/playlists", spotifyBaseURL, uid)
	body := map[string]any{
		"name":        name,
		"description": description,
		"public":      public,
	}
	var raw rawPlaylist
	if err := c.apiPost(ctx, url, body, &raw); err != nil {
		return model.Playlist{}, err
	}
	return mapRawPlaylist(raw), nil
}

// UpdatePlaylistDetails updates a playlist's name and/or description.
func (c *Client) UpdatePlaylistDetails(ctx context.Context, playlistID, name, description string) error {
	url := fmt.Sprintf("%s/playlists/%s", spotifyBaseURL, playlistID)
	body := map[string]string{
		"name":        name,
		"description": description,
	}
	return c.apiPut(ctx, url, body)
}

// DeletePlaylist unfollows (removes) a playlist from the user's library.
func (c *Client) DeletePlaylist(ctx context.Context, playlistID string) error {
	url := fmt.Sprintf("%s/playlists/%s/followers", spotifyBaseURL, playlistID)
	return c.apiDeleteWithBody(ctx, url, nil)
}

// AddTracksToPlaylist adds tracks to a playlist by their URIs.
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackURIs []string) error {
	url := fmt.Sprintf("%s/playlists/%s/items", spotifyBaseURL, playlistID)
	body := map[string]any{"uris": trackURIs}
	return c.apiPost(ctx, url, body, nil)
}

// RemoveTracksFromPlaylist removes tracks from a playlist by their URIs.
func (c *Client) RemoveTracksFromPlaylist(ctx context.Context, playlistID string, trackURIs []string) error {
	url := fmt.Sprintf("%s/playlists/%s/items", spotifyBaseURL, playlistID)
	tracks := make([]map[string]string, len(trackURIs))
	for i, uri := range trackURIs {
		tracks[i] = map[string]string{"uri": uri}
	}
	body := map[string]any{"items": tracks}
	return c.apiDeleteWithBody(ctx, url, body)
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
	c.userID = string(user.ID)
	c.mu.Unlock()
	return name, nil
}

func (c *Client) Username() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.username
}

func (c *Client) UserID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userID
}

// GetSavedTracks returns the user's liked tracks.
func (c *Client) GetSavedTracks(ctx context.Context, offset, limit int) (model.Page[model.Track], error) {
	if limit > maxPageSize {
		limit = maxPageSize
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
// Uses raw API call because the Spotify API renamed "tracks" to "items" in the response (Feb 2026).
func (c *Client) GetPlaylists(ctx context.Context, offset, limit int) (model.Page[model.Playlist], error) {
	if limit > maxPageSize {
		limit = maxPageSize
	}
	url := fmt.Sprintf("%s/me/playlists?limit=%d&offset=%d", spotifyBaseURL, limit, offset)

	var raw rawPlaylistPage
	if err := c.apiGet(ctx, url, &raw); err != nil {
		return model.Page[model.Playlist]{}, err
	}

	playlists := make([]model.Playlist, 0, len(raw.Items))
	for _, p := range raw.Items {
		playlists = append(playlists, mapRawPlaylist(p))
	}
	return model.Page[model.Playlist]{
		Items:  playlists,
		Total:  uint32(raw.Total),
		Offset: uint32(raw.Offset),
		Limit:  uint32(raw.Limit),
	}, nil
}

// GetPlaylistTracks returns tracks in a specific playlist.
// Uses /playlists/{id}/items endpoint (the old /tracks endpoint was removed Feb 2026).
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string, offset, limit int) (model.Page[model.Track], error) {
	if limit > maxPageSize {
		limit = maxPageSize
	}
	url := fmt.Sprintf("%s/playlists/%s/items?limit=%d&offset=%d", spotifyBaseURL, playlistID, limit, offset)

	var raw rawPlaylistItemsPage
	if err := c.apiGet(ctx, url, &raw); err != nil {
		return model.Page[model.Track]{}, err
	}

	tracks := make([]model.Track, 0, len(raw.Items))
	for _, item := range raw.Items {
		if item.Item != nil && item.Item.ID != "" {
			tracks = append(tracks, mapRawTrack(*item.Item))
		}
	}
	return model.Page[model.Track]{
		Items:  tracks,
		Total:  uint32(raw.Total),
		Offset: uint32(raw.Offset),
		Limit:  uint32(raw.Limit),
	}, nil
}

// GetArtistAlbums returns albums by an artist.
func (c *Client) GetArtistAlbums(ctx context.Context, artistID string, offset, limit int) (model.Page[model.Album], error) {
	if limit > maxPageSize {
		limit = maxPageSize
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
	if limit > maxPageSize {
		limit = maxPageSize
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

	searchLimit := 20 // Spotify search API max per type
	sr, err := c.sp.Search(ctx, query,
		spotifylib.SearchTypeTrack|spotifylib.SearchTypeAlbum|spotifylib.SearchTypeArtist|spotifylib.SearchTypePlaylist,
		spotifylib.Limit(searchLimit), spotifylib.Offset(off),
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
