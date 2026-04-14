package applemusic

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Apple Music library endpoints. All live under /me/library/ and
// return paginated resource collections. The generic shape is
// {"data": [resource, ...], "next": "/next-url"}. We page through
// `next` until it's empty.

// resource is the JSON shape for any Apple Music resource
// (track / playlist / album / artist). We decode into this and then
// map to our canonical types.
type resource struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Attributes map[string]any    `json:"attributes"`
	Relationships map[string]any `json:"relationships,omitempty"`
}

type paginatedResponse struct {
	Data []resource `json:"data"`
	Next string     `json:"next,omitempty"`
}

// listAll pages through a paginated endpoint and returns every
// resource. Stops on the first error or when Next is empty.
func (c *Client) listAll(ctx context.Context, path string) ([]resource, error) {
	var all []resource
	next := path
	for next != "" {
		var page paginatedResponse
		// `next` returned by Apple already includes /v1 prefix, so
		// strip it before passing to c.get which adds its own apiBase.
		if strings.HasPrefix(next, "/v1") {
			next = strings.TrimPrefix(next, "/v1")
		}
		if err := c.get(ctx, next, nil, &page); err != nil {
			return all, err
		}
		all = append(all, page.Data...)
		next = page.Next
	}
	return all, nil
}

// GetLibraryPlaylists lists every playlist in the user's library.
func (c *Client) GetLibraryPlaylists(ctx context.Context) ([]Playlist, error) {
	res, err := c.listAll(ctx, "/me/library/playlists")
	if err != nil {
		return nil, err
	}
	out := make([]Playlist, 0, len(res))
	for _, r := range res {
		pl := Playlist{ID: r.ID, Name: getStr(r.Attributes, "name")}
		// Playlist track counts aren't in the attributes on the list
		// endpoint; the caller gets them on demand via
		// GetPlaylistTracks.
		out = append(out, pl)
	}
	return out, nil
}

// GetPlaylistTracks returns every track in a specific library playlist.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string) ([]Track, error) {
	q := url.Values{}
	q.Set("limit", "100")
	res, err := c.listAll(ctx, "/me/library/playlists/"+playlistID+"/tracks?"+q.Encode())
	if err != nil {
		return nil, err
	}
	out := make([]Track, 0, len(res))
	for _, r := range res {
		t := mapTrack(r)
		if t.Title != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// GetLikedSongs returns the user's library songs (their full saved
// song list). Apple Music doesn't have a separate "liked songs" like
// Spotify; saved = added to library. For our purposes the whole
// library-songs collection is the equivalent.
func (c *Client) GetLikedSongs(ctx context.Context) ([]Track, error) {
	res, err := c.listAll(ctx, "/me/library/songs")
	if err != nil {
		return nil, err
	}
	out := make([]Track, 0, len(res))
	for _, r := range res {
		if t := mapTrack(r); t.Title != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// GetLibraryAlbums returns saved albums in the user's library.
func (c *Client) GetLibraryAlbums(ctx context.Context) ([]Album, error) {
	res, err := c.listAll(ctx, "/me/library/albums")
	if err != nil {
		return nil, err
	}
	out := make([]Album, 0, len(res))
	for _, r := range res {
		al := Album{
			ID:   r.ID,
			Name: getStr(r.Attributes, "name"),
			Year: getStr(r.Attributes, "releaseDate"),
		}
		if artist := getStr(r.Attributes, "artistName"); artist != "" {
			al.Artists = []string{artist}
		}
		if al.Name != "" {
			out = append(out, al)
		}
	}
	return out, nil
}

// GetLibraryArtists returns artists in the user's library.
func (c *Client) GetLibraryArtists(ctx context.Context) ([]Artist, error) {
	res, err := c.listAll(ctx, "/me/library/artists")
	if err != nil {
		return nil, err
	}
	out := make([]Artist, 0, len(res))
	for _, r := range res {
		ar := Artist{ID: r.ID, Name: getStr(r.Attributes, "name")}
		if ar.Name != "" {
			out = append(out, ar)
		}
	}
	return out, nil
}

// ─────── attribute helpers ───────

func getStr(attrs map[string]any, key string) string {
	v, _ := attrs[key].(string)
	return v
}

func getInt(attrs map[string]any, key string) int {
	switch v := attrs[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// mapTrack converts an Apple Music song/library-song resource into
// our canonical Track type. Apple returns artist names as a single
// pre-joined string ("Artist A & Artist B") rather than an array, so
// we split on common separators.
func mapTrack(r resource) Track {
	t := Track{
		ID:    r.ID,
		Title: getStr(r.Attributes, "name"),
		Album: getStr(r.Attributes, "albumName"),
	}
	if dur := getInt(r.Attributes, "durationInMillis"); dur > 0 {
		t.Duration = dur / 1000
	}
	if artistStr := getStr(r.Attributes, "artistName"); artistStr != "" {
		t.Artists = splitArtists(artistStr)
	}
	return t
}

// splitArtists turns "Artist A & Artist B" or "A, B & C" into a
// []string. Matching tolerates imperfect splits because the scorer
// only needs any overlap, not precise arrays.
func splitArtists(s string) []string {
	// Normalize "and" → "&" then split on " & " / ", ".
	s = strings.ReplaceAll(s, " feat. ", ", ")
	s = strings.ReplaceAll(s, " Feat. ", ", ")
	s = strings.ReplaceAll(s, " featuring ", ", ")
	s = strings.ReplaceAll(s, " and ", " & ")
	var out []string
	for _, part := range strings.Split(s, "&") {
		for _, sub := range strings.Split(part, ",") {
			if trimmed := strings.TrimSpace(sub); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}
