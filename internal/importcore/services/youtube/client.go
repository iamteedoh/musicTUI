// Package youtube wraps the YouTube Data API v3 endpoints we need
// to read a user's music library: playlists, items, liked videos.
//
// Everything takes an access token (refreshed by the caller) and
// makes direct HTTPS calls. No SDK dependency — the surface we use
// is small and Go's net/http is plenty.
//
// Quota: playlistItems.list = 1 unit, videos.list = 1 unit; the free
// daily quota is 10k. A typical 500-track library costs ~30 units.
package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/iamteedoh/musicTUI/internal/importcore/match"
)

const apiBase = "https://www.googleapis.com/youtube/v3"

// MusicCategoryID is YouTube's "Music" video category. Filtering by
// this drops personal videos, tutorials, vlogs etc. that leak into
// YT Music playlists. Deleted/private videos come back without a
// category; caller decides whether to keep them.
const MusicCategoryID = "10"

// Playlist is one YT playlist in the user's library.
type Playlist struct {
	ID          string
	Name        string
	TrackCount  int
	Description string
}

// APIError wraps a non-2xx response. Body is preserved for diagnosis.
type APIError struct {
	Status int
	Path   string
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("youtube: %s -> %d: %s", e.Path, e.Status, e.Body)
}

// Client is the YT Data API v3 wrapper. Safe to share across goroutines.
type Client struct {
	http  *http.Client
	token func(context.Context) (string, error) // pluggable token source
}

// NewClient builds a Client. `tokenFn` is called per-request to get
// a live access token — callers typically wire it to their token
// store's refresh-on-demand method.
func NewClient(tokenFn func(context.Context) (string, error)) *Client {
	return &Client{
		http:  &http.Client{},
		token: tokenFn,
	}
}

// ListPlaylists returns every playlist the authenticated user owns
// via /playlists?mine=true, paginated.
func (c *Client) ListPlaylists(ctx context.Context) ([]Playlist, error) {
	var out []Playlist
	pageToken := ""
	for {
		params := url.Values{
			"part":       []string{"snippet,contentDetails"},
			"mine":       []string{"true"},
			"maxResults": []string{"50"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var body playlistsListResp
		if err := c.get(ctx, "/playlists", params, &body); err != nil {
			return nil, err
		}
		for _, item := range body.Items {
			out = append(out, Playlist{
				ID:          item.ID,
				Name:        item.Snippet.Title,
				Description: item.Snippet.Description,
				TrackCount:  item.ContentDetails.ItemCount,
			})
		}
		if body.NextPageToken == "" {
			return out, nil
		}
		pageToken = body.NextPageToken
	}
}

// ListPlaylistItems returns every video in `playlistID` as a
// match.Track. Two-phase fetch: /playlistItems for video IDs, then
// /videos in 50-ID batches for canonical title/channel/duration/
// categoryId metadata. Videos missing from /videos (deleted, private,
// region-locked) fall back to the playlist-snippet title and owner.
func (c *Client) ListPlaylistItems(ctx context.Context, playlistID string) ([]match.Track, error) {
	var videoIDs []string
	fallbackArtists := map[string]string{}
	fallbackTitles := map[string]string{}

	pageToken := ""
	for {
		params := url.Values{
			"part":       []string{"snippet,contentDetails"},
			"playlistId": []string{playlistID},
			"maxResults": []string{"50"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var body playlistItemsResp
		if err := c.get(ctx, "/playlistItems", params, &body); err != nil {
			return nil, err
		}
		for _, item := range body.Items {
			vid := item.ContentDetails.VideoID
			if vid == "" {
				continue
			}
			videoIDs = append(videoIDs, vid)
			fallbackTitles[vid] = item.Snippet.Title
			if item.Snippet.VideoOwnerChannelTitle != "" {
				fallbackArtists[vid] = item.Snippet.VideoOwnerChannelTitle
			}
		}
		if body.NextPageToken == "" {
			break
		}
		pageToken = body.NextPageToken
	}

	meta, err := c.videosByID(ctx, videoIDs)
	if err != nil {
		return nil, err
	}

	tracks := make([]match.Track, 0, len(videoIDs))
	for _, vid := range videoIDs {
		info, ok := meta[vid]
		if !ok {
			// Deleted / private / region-locked — use playlist snippet.
			artists := []string(nil)
			if a, ok := fallbackArtists[vid]; ok {
				artists = []string{a}
			}
			tracks = append(tracks, match.Track{
				ID:      vid,
				Title:   fallbackTitles[vid],
				Artists: artists,
			})
			continue
		}
		tracks = append(tracks, toTrack(vid, info))
	}
	return tracks, nil
}

// ListLikedVideos returns the user's liked videos (the special "LL"
// playlist) as match.Track. Uses /videos?myRating=like directly
// rather than going through /playlistItems — one fewer API hop and
// returns full metadata immediately.
func (c *Client) ListLikedVideos(ctx context.Context) ([]match.Track, error) {
	var out []match.Track
	pageToken := ""
	for {
		params := url.Values{
			"part":       []string{"snippet,contentDetails"},
			"myRating":   []string{"like"},
			"maxResults": []string{"50"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var body videosListResp
		if err := c.get(ctx, "/videos", params, &body); err != nil {
			return nil, err
		}
		for _, item := range body.Items {
			out = append(out, toTrack(item.ID, item))
		}
		if body.NextPageToken == "" {
			return out, nil
		}
		pageToken = body.NextPageToken
	}
}

// Category returns the raw categoryId for a list of video IDs —
// useful when the caller needs to filter a pre-existing Track slice
// by music-only. Most callers shouldn't need this (ListPlaylistItems
// already embeds categoryId via a detour), but it's exposed as an
// escape hatch.
//
// Callers that want music-only can filter directly on
// match.Track-with-extras by reading the CategoryID field populated
// by toTrack.

// ─────────────────── internals ───────────────────

func (c *Client) videosByID(ctx context.Context, ids []string) (map[string]videoItem, error) {
	out := map[string]videoItem{}
	for i := 0; i < len(ids); i += 50 {
		end := i + 50
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]
		params := url.Values{
			"part": []string{"snippet,contentDetails"},
			"id":   []string{strings.Join(chunk, ",")},
		}
		var body videosListResp
		if err := c.get(ctx, "/videos", params, &body); err != nil {
			return nil, err
		}
		for _, item := range body.Items {
			out[item.ID] = item
		}
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("youtube token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &APIError{Status: resp.StatusCode, Path: path, Body: string(body)}
	}
	return json.Unmarshal(body, out)
}

// toTrack normalizes a YT video into a match.Track. Strips the
// " - Topic" suffix on YT Music artist channels so names match
// Spotify's canonical form.
func toTrack(id string, v videoItem) match.Track {
	artist := stripTopicSuffix(v.Snippet.ChannelTitle)
	artists := []string(nil)
	if artist != "" {
		artists = []string{artist}
	}
	return match.Track{
		ID:       id,
		Title:    v.Snippet.Title,
		Artists:  artists,
		Duration: parseISO8601Seconds(v.ContentDetails.Duration),
	}
}

// CategoryID returns the YT category for a video. Separate helper so
// callers can filter a track list after construction without mixing
// it into the match.Track shape (match should stay source-agnostic).
func CategoryID(v videoItem) string {
	return v.Snippet.CategoryID
}

// FetchCategories is a batched categoryId lookup for a set of video
// IDs. Returns a {videoID -> categoryId} map. The importer uses this
// to filter non-music out of playlists that mix content.
func (c *Client) FetchCategories(ctx context.Context, ids []string) (map[string]string, error) {
	meta, err := c.videosByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(meta))
	for id, v := range meta {
		out[id] = v.Snippet.CategoryID
	}
	return out, nil
}

// ListPlaylistItemsWithCategory is a convenience combining
// ListPlaylistItems with a categoryId lookup — so the caller gets
// tracks + a parallel category slice in one call. Simplifies the
// music-only filter in the importer.
func (c *Client) ListPlaylistItemsWithCategory(ctx context.Context, playlistID string) ([]match.Track, []string, error) {
	// Two-phase fetch inlined: the existing ListPlaylistItems already
	// calls videosByID, so we repeat the shape here and pull category
	// out of the same request without a second round-trip.
	var videoIDs []string
	fallbackArtists := map[string]string{}
	fallbackTitles := map[string]string{}
	pageToken := ""
	for {
		params := url.Values{
			"part":       []string{"snippet,contentDetails"},
			"playlistId": []string{playlistID},
			"maxResults": []string{"50"},
		}
		if pageToken != "" {
			params.Set("pageToken", pageToken)
		}
		var body playlistItemsResp
		if err := c.get(ctx, "/playlistItems", params, &body); err != nil {
			return nil, nil, err
		}
		for _, item := range body.Items {
			vid := item.ContentDetails.VideoID
			if vid == "" {
				continue
			}
			videoIDs = append(videoIDs, vid)
			fallbackTitles[vid] = item.Snippet.Title
			if item.Snippet.VideoOwnerChannelTitle != "" {
				fallbackArtists[vid] = item.Snippet.VideoOwnerChannelTitle
			}
		}
		if body.NextPageToken == "" {
			break
		}
		pageToken = body.NextPageToken
	}

	meta, err := c.videosByID(ctx, videoIDs)
	if err != nil {
		return nil, nil, err
	}

	tracks := make([]match.Track, 0, len(videoIDs))
	cats := make([]string, 0, len(videoIDs))
	for _, vid := range videoIDs {
		info, ok := meta[vid]
		if !ok {
			artists := []string(nil)
			if a, ok := fallbackArtists[vid]; ok {
				artists = []string{a}
			}
			tracks = append(tracks, match.Track{
				ID:      vid,
				Title:   fallbackTitles[vid],
				Artists: artists,
			})
			cats = append(cats, "") // unknown — let caller decide
			continue
		}
		tracks = append(tracks, toTrack(vid, info))
		cats = append(cats, info.Snippet.CategoryID)
	}
	return tracks, cats, nil
}

// ─────────────────── small string helpers ───────────────────

func stripTopicSuffix(channel string) string {
	const suffix = " - Topic"
	if strings.HasSuffix(channel, suffix) {
		return strings.TrimSpace(channel[:len(channel)-len(suffix)])
	}
	return strings.TrimSpace(channel)
}

// parseISO8601Seconds handles the narrow "PT{H}H{M}M{S}S" format
// YouTube returns for video durations. A full ISO-8601 parser would
// be overkill.
func parseISO8601Seconds(s string) int {
	if !strings.HasPrefix(s, "PT") {
		return 0
	}
	s = s[2:]
	var hours, minutes, seconds int
	var numStr strings.Builder
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			numStr.WriteRune(ch)
			continue
		}
		if numStr.Len() == 0 {
			continue
		}
		n, err := strconv.Atoi(numStr.String())
		numStr.Reset()
		if err != nil {
			return 0
		}
		switch ch {
		case 'H':
			hours = n
		case 'M':
			minutes = n
		case 'S':
			seconds = n
		}
	}
	return hours*3600 + minutes*60 + seconds
}

// ─────────────────── JSON shapes ───────────────────

type playlistsListResp struct {
	NextPageToken string         `json:"nextPageToken"`
	Items         []playlistItem `json:"items"`
}

type playlistItem struct {
	ID      string `json:"id"`
	Snippet struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"snippet"`
	ContentDetails struct {
		ItemCount int `json:"itemCount"`
	} `json:"contentDetails"`
}

type playlistItemsResp struct {
	NextPageToken string                 `json:"nextPageToken"`
	Items         []playlistItemResource `json:"items"`
}

type playlistItemResource struct {
	Snippet struct {
		Title                  string `json:"title"`
		VideoOwnerChannelTitle string `json:"videoOwnerChannelTitle"`
	} `json:"snippet"`
	ContentDetails struct {
		VideoID string `json:"videoId"`
	} `json:"contentDetails"`
}

type videosListResp struct {
	NextPageToken string      `json:"nextPageToken"`
	Items         []videoItem `json:"items"`
}

type videoItem struct {
	ID      string `json:"id"`
	Snippet struct {
		Title        string `json:"title"`
		ChannelTitle string `json:"channelTitle"`
		CategoryID   string `json:"categoryId"`
	} `json:"snippet"`
	ContentDetails struct {
		Duration string `json:"duration"`
	} `json:"contentDetails"`
}
