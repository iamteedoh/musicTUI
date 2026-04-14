package ytmusic

import (
	"context"
	"strconv"
	"strings"
)

// Library browseIds. These are stable identifiers in YT Music's
// internal taxonomy — they have not changed in the lifetime of the
// web client as far as ytmusicapi's history shows. If YT ever renames
// them, that's a separate rev.
const (
	browseLibraryPlaylists = "FEmusic_liked_playlists"
	browseLibraryLiked     = "FEmusic_liked_videos"
	browseLibraryAlbums    = "FEmusic_liked_albums"
	browseLibraryArtists   = "FEmusic_library_corpus_track_artists"
)

// GetLibraryPlaylists returns every playlist in the user's library
// (ones they created and ones they follow — YT Music doesn't
// distinguish, which actually mirrors Spotify's concept nicely).
func (c *Client) GetLibraryPlaylists(ctx context.Context) ([]Playlist, error) {
	resp, err := c.browse(ctx, map[string]any{"browseId": browseLibraryPlaylists})
	if err != nil {
		return nil, err
	}
	items := findShelfItems(resp)
	out := make([]Playlist, 0, len(items))
	for _, item := range items {
		// The library playlists page uses gridRenderer with
		// musicTwoRowItemRenderer entries.
		two := asMap(walk(item, "musicTwoRowItemRenderer"))
		if two != nil {
			if pl := parsePlaylistTile(two); pl.ID != "" {
				out = append(out, pl)
			}
			continue
		}
		// Fallback for musicResponsiveListItemRenderer shape.
		list := asMap(walk(item, "musicResponsiveListItemRenderer"))
		if list != nil {
			if pl := parsePlaylistRow(list); pl.ID != "" {
				out = append(out, pl)
			}
		}
	}
	return out, nil
}

// GetLikedSongs returns the user's liked music as Tracks. Note YT
// Music surfaces "Liked Music" as a playlist with a fixed ID; we just
// fetch that playlist's items directly rather than going through a
// separate liked-videos browse call.
func (c *Client) GetLikedSongs(ctx context.Context) ([]Track, error) {
	// Liked Music is an auto-generated playlist keyed "LM".
	return c.GetPlaylistTracks(ctx, "LM")
}

// GetLibraryAlbums returns saved albums from the user's library.
func (c *Client) GetLibraryAlbums(ctx context.Context) ([]Album, error) {
	resp, err := c.browse(ctx, map[string]any{"browseId": browseLibraryAlbums})
	if err != nil {
		return nil, err
	}
	items := findShelfItems(resp)
	out := make([]Album, 0, len(items))
	for _, item := range items {
		two := asMap(walk(item, "musicTwoRowItemRenderer"))
		if two == nil {
			continue
		}
		al := parseAlbumTile(two)
		if al.BrowseID != "" {
			out = append(out, al)
		}
	}
	return out, nil
}

// GetLibraryArtists returns the artists the user follows in their
// library. YT Music returns these as list items with a channelId
// under the title's navigation endpoint.
func (c *Client) GetLibraryArtists(ctx context.Context) ([]Artist, error) {
	resp, err := c.browse(ctx, map[string]any{"browseId": browseLibraryArtists})
	if err != nil {
		return nil, err
	}
	items := findShelfItems(resp)
	out := make([]Artist, 0, len(items))
	for _, item := range items {
		list := asMap(walk(item, "musicResponsiveListItemRenderer"))
		if list == nil {
			continue
		}
		ar := parseArtistRow(list)
		if ar.ChannelID != "" {
			out = append(out, ar)
		}
	}
	return out, nil
}

// GetPlaylistTracks fetches the tracks inside a single playlist by ID.
// The internal endpoint for a playlist is `browse` with browseId
// prefixed "VL" — e.g. GetPlaylistTracks("PLxxxx") sends "VLPLxxxx".
// The special "LM" (Liked Music) playlist is also a "VL" target.
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string) ([]Track, error) {
	browseID := playlistID
	if !strings.HasPrefix(browseID, "VL") {
		browseID = "VL" + browseID
	}
	resp, err := c.browse(ctx, map[string]any{"browseId": browseID})
	if err != nil {
		return nil, err
	}
	// Playlist pages nest tracks at
	//   contents.singleColumnBrowseResultsRenderer.tabs[0].tabRenderer.
	//     content.sectionListRenderer.contents[0].musicPlaylistShelfRenderer.contents
	section := walk(resp,
		"contents", "singleColumnBrowseResultsRenderer",
		"tabs", 0, "tabRenderer",
		"content", "sectionListRenderer",
		"contents", 0)
	items := asSlice(walk(section, "musicPlaylistShelfRenderer", "contents"))
	if items == nil {
		items = findShelfItems(resp)
	}
	out := make([]Track, 0, len(items))
	for _, item := range items {
		list := asMap(walk(item, "musicResponsiveListItemRenderer"))
		if list == nil {
			continue
		}
		if t := parseTrackRow(list); t.Title != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// ───────────────────── renderer parsers ─────────────────────

// parsePlaylistTile handles musicTwoRowItemRenderer, the tile shape
// used by the library-playlists grid page. Title is a single run with
// the click target being a browseEndpoint whose browseId IS the
// playlist ID (for VL-prefixed library IDs, we strip the prefix).
func parsePlaylistTile(r map[string]any) Playlist {
	titleRun := walk(r, "title", "runs", 0)
	id := navBrowseID(titleRun)
	id = strings.TrimPrefix(id, "VL")
	return Playlist{
		ID:        id,
		Name:      firstRunText(walk(r, "title", "runs")),
		Thumbnail: firstThumbURL(walk(r, "thumbnailRenderer")),
	}
}

// parsePlaylistRow handles musicResponsiveListItemRenderer, used by
// some YT variants for the library playlist list. Track count lives
// in the second flex column's runs.
func parsePlaylistRow(r map[string]any) Playlist {
	cols := asSlice(walk(r, "flexColumns"))
	if len(cols) == 0 {
		return Playlist{}
	}
	titleRuns := walk(cols[0],
		"musicResponsiveListItemFlexColumnRenderer", "text", "runs")
	pl := Playlist{
		Name:      firstRunText(titleRuns),
		ID:        strings.TrimPrefix(navBrowseID(walk(titleRuns, 0)), "VL"),
		Thumbnail: firstThumbURL(walk(r, "thumbnail")),
	}
	if len(cols) > 1 {
		// Format: "Playlist · 42 tracks" or just "42 tracks"
		meta := runsText(walk(cols[1],
			"musicResponsiveListItemFlexColumnRenderer", "text", "runs"))
		pl.TrackCount = parseTrackCountSuffix(meta)
	}
	return pl
}

func parseTrackRow(r map[string]any) Track {
	cols := asSlice(walk(r, "flexColumns"))
	t := Track{}
	if len(cols) > 0 {
		titleRuns := asSlice(walk(cols[0],
			"musicResponsiveListItemFlexColumnRenderer", "text", "runs"))
		t.Title = firstRunText(titleRuns)
		t.VideoID = asString(walk(titleRuns, 0,
			"navigationEndpoint", "watchEndpoint", "videoId"))
	}
	if len(cols) > 1 {
		// Second column: "Artist A & Artist B · Album · 3:42"
		runs := asSlice(walk(cols[1],
			"musicResponsiveListItemFlexColumnRenderer", "text", "runs"))
		t.Artists, t.Album = parseArtistsAndAlbum(runs)
	}
	// Duration is often in a fixed column (3rd) or in the track's
	// `fixedColumns`. Best-effort; 0 is fine.
	fixed := asSlice(walk(r, "fixedColumns"))
	if len(fixed) > 0 {
		t.Duration = parseDuration(runsText(walk(fixed[0],
			"musicResponsiveListItemFixedColumnRenderer", "text", "runs")))
	}
	return t
}

func parseAlbumTile(r map[string]any) Album {
	titleRuns := walk(r, "title", "runs")
	al := Album{
		BrowseID: navBrowseID(walk(titleRuns, 0)),
		Name:     firstRunText(titleRuns),
	}
	// Subtitle runs: "Album · Artist · 2019"
	subRuns := asSlice(walk(r, "subtitle", "runs"))
	for i, run := range subRuns {
		txt := asString(walk(run, "text"))
		if txt == "" || txt == " · " {
			continue
		}
		if i == 0 { // usually "Album" / "EP" / "Single"
			continue
		}
		// If this run has a browse endpoint, it's an artist name; otherwise it's year.
		if navBrowseID(run) != "" {
			al.Artists = append(al.Artists, txt)
		} else if isYear(txt) {
			al.Year = txt
		}
	}
	return al
}

func parseArtistRow(r map[string]any) Artist {
	cols := asSlice(walk(r, "flexColumns"))
	if len(cols) == 0 {
		return Artist{}
	}
	nameRuns := asSlice(walk(cols[0],
		"musicResponsiveListItemFlexColumnRenderer", "text", "runs"))
	return Artist{
		Name:      firstRunText(nameRuns),
		ChannelID: navBrowseID(walk(nameRuns, 0)),
	}
}

// ───────────────────── tiny string helpers ─────────────────────

// parseArtistsAndAlbum splits a YT subtitle runs array into
// artist names and album name. YT separates with " · " runs; artists
// each have a navigationEndpoint of type browseEndpoint, the album
// does too but its browseId starts "MPREb_". Order is
// [artist][ · ][artist][ · ][album][ · ][duration] typically.
func parseArtistsAndAlbum(runs []any) (artists []string, album string) {
	for _, run := range runs {
		txt := asString(walk(run, "text"))
		if txt == "" || strings.TrimSpace(txt) == "·" {
			continue
		}
		bid := navBrowseID(run)
		switch {
		case strings.HasPrefix(bid, "UC"): // channel = artist
			artists = append(artists, txt)
		case strings.HasPrefix(bid, "MPREb_"): // album browseId
			album = txt
		}
	}
	return
}

func parseTrackCountSuffix(s string) int {
	// Extract the first integer in the string — handles "42 tracks",
	// "Playlist · 42 songs", etc.
	var num string
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
		} else if num != "" {
			break
		}
	}
	if num == "" {
		return 0
	}
	n, _ := strconv.Atoi(num)
	return n
}

func parseDuration(s string) int {
	// "3:42" → 222 ; "1:23:45" → 5025. Anything weird returns 0.
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	var total int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

func isYear(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func firstThumbURL(renderer any) string {
	thumbs := asSlice(walk(renderer,
		"musicThumbnailRenderer", "thumbnail", "thumbnails"))
	if len(thumbs) == 0 {
		thumbs = asSlice(walk(renderer, "thumbnail", "thumbnails"))
	}
	if len(thumbs) == 0 {
		return ""
	}
	return asString(walk(thumbs[len(thumbs)-1], "url"))
}
