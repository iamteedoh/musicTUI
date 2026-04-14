package ytmusic

import (
	"context"
	"fmt"
	"strings"

	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
)

// Spotify's add-tracks API caps at 100 URIs per call. The limit is
// official and hard — not rate limit but a request-size limit.
const spotifyAddTracksBatchSize = 100

// ImportOptions captures the user's choices for a single playlist
// import: what to name the resulting Spotify playlist and whether to
// make it public.
type ImportOptions struct {
	Name        string
	Description string
	Public      bool
}

// ImportSummary is what the UI shows when an import finishes.
type ImportSummary struct {
	Playlist       model.Playlist
	Matches        []Match // parallel to the input tracks, includes misses
	MatchedCount   int
	UnmatchedCount int
	ErrorCount     int
}

// ImportPlaylist runs the full flow for one YT Music playlist:
//  1. match every track to Spotify
//  2. create a new Spotify playlist
//  3. add the matched tracks in batches of 100
//
// progress is called after each per-track match (for a UI progress
// bar) and again once with done==total+1 right before the Spotify
// write phase, so the UI can show "matching" → "creating playlist"
// → "adding tracks". nil disables progress reporting.
//
// Non-fatal per-track errors (e.g. a single Spotify search fails) are
// recorded as Match.Error in the summary and counted in ErrorCount;
// the import still proceeds. Fatal errors (can't create the playlist,
// can't add any batch) are returned directly.
func ImportPlaylist(
	ctx context.Context,
	spClient *sp.Client,
	tracks []Track,
	opts ImportOptions,
	progress func(done, total int),
) (*ImportSummary, error) {
	matches := MatchTracks(ctx, spClient, tracks, progress)

	pl, err := spClient.CreatePlaylist(ctx, opts.Name, opts.Description, opts.Public)
	if err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}

	summary := &ImportSummary{
		Playlist: pl,
		Matches:  matches,
	}

	uris := make([]string, 0, len(matches))
	for _, m := range matches {
		switch {
		case m.Error != nil:
			summary.ErrorCount++
		case m.URI() != "":
			uris = append(uris, m.URI())
			summary.MatchedCount++
		default:
			summary.UnmatchedCount++
		}
	}

	for i := 0; i < len(uris); i += spotifyAddTracksBatchSize {
		end := i + spotifyAddTracksBatchSize
		if end > len(uris) {
			end = len(uris)
		}
		if err := spClient.AddTracksToPlaylist(ctx, pl.ID, uris[i:end]); err != nil {
			return summary, fmt.Errorf("add tracks batch %d-%d: %w", i, end, err)
		}
	}

	return summary, nil
}

// ImportPlaylists applies ImportPlaylist to every playlist in the
// user's YT Music library in sequence. The naming convention prefixes
// every imported playlist with "[YT] " so the user can tell imported
// from native at a glance and cleanup is easy if an import goes wrong.
func ImportPlaylists(
	ctx context.Context,
	yt *Client,
	spClient *sp.Client,
	progress func(playlistName string, done, total int, overall int, overallTotal int),
) ([]ImportSummary, error) {
	playlists, err := yt.GetLibraryPlaylists(ctx)
	if err != nil {
		return nil, fmt.Errorf("list YT playlists: %w", err)
	}
	summaries := make([]ImportSummary, 0, len(playlists))
	for pi, pl := range playlists {
		if ctx.Err() != nil {
			return summaries, ctx.Err()
		}
		tracks, err := yt.GetPlaylistTracks(ctx, pl.ID)
		if err != nil {
			// Skip playlists we can't load but keep going.
			summaries = append(summaries, ImportSummary{
				Playlist: model.Playlist{Name: pl.Name},
				Matches:  nil,
			})
			continue
		}
		name := "[YT] " + pl.Name
		opts := ImportOptions{
			Name:        name,
			Description: fmt.Sprintf("Imported from YouTube Music. Source playlist: %q.", pl.Name),
			Public:      false,
		}
		trackProgress := func(done, total int) {
			if progress != nil {
				progress(pl.Name, done, total, pi+1, len(playlists))
			}
		}
		s, err := ImportPlaylist(ctx, spClient, tracks, opts, trackProgress)
		if err != nil {
			// Bubble up fatal playlist errors but keep partial summaries.
			if s != nil {
				summaries = append(summaries, *s)
			}
			return summaries, fmt.Errorf("import %q: %w", pl.Name, err)
		}
		summaries = append(summaries, *s)
	}
	return summaries, nil
}

// ImportLikedSongs imports the user's YT Music "Liked Music" into a
// fresh Spotify playlist. Spotify's "Liked Songs" is a special list
// the Web API can only modify via the "Save Tracks for Current User"
// endpoint (PUT /me/tracks), which our internal/spotify client doesn't
// currently wrap. For the first cut we create a regular playlist
// called "[YT] Liked Music" so the user still gets their songs in one
// place; a future rev can gate "merge into Spotify Liked Songs?" on
// the add-tracks scope.
func ImportLikedSongs(
	ctx context.Context,
	yt *Client,
	spClient *sp.Client,
	progress func(done, total int),
) (*ImportSummary, error) {
	tracks, err := yt.GetLikedSongs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list liked songs: %w", err)
	}
	opts := ImportOptions{
		Name:        "[YT] Liked Music",
		Description: "Imported from YouTube Music — your Liked Music playlist.",
		Public:      false,
	}
	return ImportPlaylist(ctx, spClient, tracks, opts, progress)
}

// ImportLibraryAlbums takes the user's saved YT Music albums and
// creates one Spotify playlist per album, titled "[YT Album] <Album>".
// Spotify has a proper "saved albums" concept, but saving a Spotify
// album requires knowing the Spotify album ID — which means we'd
// need to search & match every album, and an album-level match is
// much noisier than a track-level match. Playlist per album is a
// lossier but simpler first pass; we can add real "Save to library"
// behavior once album matching is good enough.
func ImportLibraryAlbums(
	ctx context.Context,
	yt *Client,
	spClient *sp.Client,
	progress func(albumName string, done, total int, overall, overallTotal int),
) ([]ImportSummary, error) {
	albums, err := yt.GetLibraryAlbums(ctx)
	if err != nil {
		return nil, fmt.Errorf("list library albums: %w", err)
	}
	// For now: album-level import isn't implemented end-to-end —
	// we only surface the album list so the caller can present it
	// and let the user pick which to import as playlists. This
	// function is a placeholder for Phase 3.5 (album track fetching
	// requires a separate InnerTube browse for the album itself).
	_ = albums
	_ = spClient
	_ = progress
	return nil, fmt.Errorf("library-album import not yet implemented — coming in next revision")
}

// normalizePlaylistName keeps the target Spotify playlist name under
// Spotify's 100-char hard limit while preserving the "[YT] " prefix
// signal. Not used yet — kept here for when Phase 4 lets the user
// rename per-playlist before import.
func normalizePlaylistName(s string) string {
	s = strings.TrimSpace(s)
	const spotifyMax = 100
	if len(s) > spotifyMax {
		s = s[:spotifyMax-1] + "…"
	}
	return s
}
