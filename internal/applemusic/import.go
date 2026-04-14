package applemusic

import (
	"context"
	"fmt"

	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
	"github.com/iamteedoh/musicTUI/internal/ytmusic"
)

// Import reuses internal/ytmusic's matching engine by converting
// Apple Music tracks into the YT Music Track shape before matching.
// Title + Artists are the only fields the matcher reads, so this
// shim is safe. A future refactor could extract the matching logic
// to internal/importer/ to avoid this ytmusic import; for now,
// pragmatic code reuse beats a refactor mid-feature.

// ImportSummary mirrors ytmusic.ImportSummary exactly — same shape,
// reused to keep the UI code uniform across both import sources.
type ImportSummary = ytmusic.ImportSummary

// Spotify's add-tracks API caps at 100 URIs per call.
const spotifyAddTracksBatchSize = 100

// ImportPlaylists imports every library playlist as a "[AM] <name>"
// Spotify playlist.
func ImportPlaylists(
	ctx context.Context,
	am *Client,
	spClient *sp.Client,
) ([]ImportSummary, error) {
	playlists, err := am.GetLibraryPlaylists(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Apple Music playlists: %w", err)
	}
	summaries := make([]ImportSummary, 0, len(playlists))
	for _, pl := range playlists {
		if ctx.Err() != nil {
			return summaries, ctx.Err()
		}
		tracks, err := am.GetPlaylistTracks(ctx, pl.ID)
		if err != nil {
			summaries = append(summaries, ImportSummary{
				Playlist: model.Playlist{Name: pl.Name},
			})
			continue
		}
		s, err := importTracksToSpotify(ctx, spClient,
			tracks, "[AM] "+pl.Name,
			"Imported from Apple Music. Source playlist: \""+pl.Name+"\".")
		if err != nil {
			if s != nil {
				summaries = append(summaries, *s)
			}
			return summaries, fmt.Errorf("import %q: %w", pl.Name, err)
		}
		summaries = append(summaries, *s)
	}
	return summaries, nil
}

// ImportLikedSongs dumps the user's library-songs collection into a
// single Spotify playlist called "[AM] Library Songs". Same
// reasoning as the YT import's Liked: merging into Spotify's "Liked
// Songs" would need an extra scope and opt-in.
func ImportLikedSongs(ctx context.Context, am *Client, spClient *sp.Client) (*ImportSummary, error) {
	tracks, err := am.GetLikedSongs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list library songs: %w", err)
	}
	return importTracksToSpotify(ctx, spClient, tracks,
		"[AM] Library Songs",
		"Imported from Apple Music — your full library.")
}

// importTracksToSpotify runs the match → create playlist → add tracks
// flow. Mirrors ytmusic.ImportPlaylist but takes Apple Music Track
// values directly (converted to ytmusic.Track shape for matching).
func importTracksToSpotify(
	ctx context.Context,
	spClient *sp.Client,
	tracks []Track,
	playlistName, description string,
) (*ImportSummary, error) {
	matches := make([]ytmusic.Match, len(tracks))
	for i, t := range tracks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Convert to ytmusic.Track for matching. Only Title + Artists
		// are read by MatchTrack — safe to fake VideoID as the Apple
		// ID so error reporting still identifies the source track.
		yt := ytmusic.Track{
			VideoID: t.ID,
			Title:   t.Title,
			Artists: t.Artists,
			Album:   t.Album,
		}
		matches[i] = ytmusic.MatchTrack(ctx, spClient, yt)
	}

	pl, err := spClient.CreatePlaylist(ctx, playlistName, description, false)
	if err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}

	summary := &ImportSummary{Playlist: pl, Matches: matches}
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
			return summary, fmt.Errorf("add tracks: %w", err)
		}
	}
	return summary, nil
}
