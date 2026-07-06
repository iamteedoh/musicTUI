package importer

import (
	"context"
	"fmt"

	"github.com/iamteedoh/musicTUI/internal/importcore/match"
	"github.com/iamteedoh/musicTUI/internal/importcore/services/spotify"
	"github.com/iamteedoh/musicTUI/internal/importcore/services/youtube"
)

// Request captures user choices for one import run. Mirrors the
// hosted backend's POST /import body so the surface is familiar.
type Request struct {
	Source       string   // "youtube" (only supported today)
	Dest         string   // "spotify"
	PlaylistIDs  []string // empty → all playlists
	IncludeLiked bool     // off by default; see note in the readme
}

// Run executes an import end-to-end. Returns a read-only channel of
// Events; the channel closes when the job ends (success, error, or
// ctx cancellation). Callers should range over it.
//
// The work runs in a goroutine so the caller can read events live
// and doesn't block on setup — identical shape to the hosted
// backend's SSE stream, just with Go channels instead of HTTP.
func Run(ctx context.Context, yt *youtube.Client, sp *spotify.Client, req Request) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		run(ctx, yt, sp, req, out)
	}()
	return out
}

func run(ctx context.Context, yt *youtube.Client, sp *spotify.Client, req Request, out chan<- Event) {
	emit(ctx, out, Event{Type: EventJobStarted, Source: req.Source, Dest: req.Dest})

	user, err := sp.Whoami(ctx)
	if err != nil {
		emit(ctx, out, Event{Type: EventError, Message: fmt.Sprintf("spotify whoami: %v", err)})
		return
	}
	_ = user // available for debug logging; not user-visible yet

	// Snapshot existing playlist names so we skip re-creating ones that
	// already exist. Imports stay idempotent — safe to re-run without
	// producing duplicates.
	existing, err := sp.ListMyPlaylistNames(ctx)
	if err != nil {
		emit(ctx, out, Event{Type: EventError, Message: fmt.Sprintf("list existing playlists: %v", err)})
		return
	}

	playlists, err := yt.ListPlaylists(ctx)
	if err != nil {
		emit(ctx, out, Event{Type: EventError, Message: fmt.Sprintf("list yt playlists: %v", err)})
		return
	}
	if len(req.PlaylistIDs) > 0 {
		wanted := make(map[string]struct{}, len(req.PlaylistIDs))
		for _, id := range req.PlaylistIDs {
			wanted[id] = struct{}{}
		}
		filtered := playlists[:0]
		for _, p := range playlists {
			if _, ok := wanted[p.ID]; ok {
				filtered = append(filtered, p)
			}
		}
		playlists = filtered
	}

	var totalMatched, totalUnmatched, totalErrors, processed int

	for _, pl := range playlists {
		if ctx.Err() != nil {
			return
		}
		tracks, cats, err := yt.ListPlaylistItemsWithCategory(ctx, pl.ID)
		if err != nil {
			totalErrors++
			emit(ctx, out, Event{
				Type:    EventError,
				Message: fmt.Sprintf("load playlist %q: %v", pl.Name, err),
			})
			continue
		}
		music := filterMusic(tracks, cats)
		filteredOut := len(tracks) - len(music)

		if len(music) == 0 {
			emit(ctx, out, Event{
				Type:             EventPlaylistSkipped,
				PlaylistName:     pl.Name,
				SkipReason:       "no music tracks (all filtered by category)",
				FilteredNonMusic: filteredOut,
			})
			continue
		}
		if _, exists := existing[pl.Name]; exists {
			emit(ctx, out, Event{
				Type:             EventPlaylistSkipped,
				PlaylistName:     pl.Name,
				SkipReason:       "already exists in Spotify",
				FilteredNonMusic: filteredOut,
			})
			continue
		}

		emit(ctx, out, Event{
			Type:             EventPlaylistStarted,
			PlaylistName:     pl.Name,
			PlaylistTotal:    len(music),
			FilteredNonMusic: filteredOut,
		})

		matched, unmatched, errors, url := importTracks(ctx, sp, pl.Name, music, out)
		totalMatched += matched
		totalUnmatched += unmatched
		totalErrors += errors
		processed++
		existing[pl.Name] = struct{}{} // subsequent YT playlists with same name dedupe too

		emit(ctx, out, Event{
			Type:             EventPlaylistDone,
			PlaylistName:     pl.Name,
			PlaylistURL:      url,
			Matched:          matched,
			Unmatched:        unmatched,
			Errors:           errors,
			FilteredNonMusic: filteredOut,
		})
	}

	// Liked videos — opt-in, also music-filtered.
	if req.IncludeLiked {
		liked, err := yt.ListLikedVideos(ctx)
		if err != nil {
			totalErrors++
			emit(ctx, out, Event{Type: EventError, Message: fmt.Sprintf("list liked videos: %v", err)})
		} else {
			// Liked videos path doesn't have per-track categories yet —
			// filter by calling back to the categories endpoint. Cheap
			// (1 unit per 50 IDs) and worth it for correctness.
			ids := make([]string, len(liked))
			for i, t := range liked {
				ids[i] = t.ID
			}
			cats, err := yt.FetchCategories(ctx, ids)
			if err != nil {
				totalErrors++
				emit(ctx, out, Event{Type: EventError, Message: fmt.Sprintf("fetch liked categories: %v", err)})
			} else {
				parallel := make([]string, len(liked))
				for i, t := range liked {
					parallel[i] = cats[t.ID]
				}
				music := filterMusic(liked, parallel)
				if len(music) > 0 {
					name := "Liked Music"
					if _, exists := existing[name]; exists {
						emit(ctx, out, Event{
							Type:         EventPlaylistSkipped,
							PlaylistName: name,
							SkipReason:   "already exists in Spotify",
						})
					} else {
						emit(ctx, out, Event{
							Type:          EventPlaylistStarted,
							PlaylistName:  name,
							PlaylistTotal: len(music),
						})
						matched, unmatched, errs, url := importTracks(ctx, sp, name, music, out)
						totalMatched += matched
						totalUnmatched += unmatched
						totalErrors += errs
						processed++
						emit(ctx, out, Event{
							Type:         EventPlaylistDone,
							PlaylistName: name,
							PlaylistURL:  url,
							Matched:      matched,
							Unmatched:    unmatched,
							Errors:       errs,
						})
					}
				}
			}
		}
	}

	emit(ctx, out, Event{
		Type:          EventJobDone,
		PlaylistCount: processed,
		Matched:       totalMatched,
		Unmatched:     totalUnmatched,
		Errors:        totalErrors,
	})
}

// importTracks handles the per-playlist work: create the Spotify
// playlist, search + score each track, emit per-track events, add
// matched URIs in 100-batches. Returns aggregate counts + the new
// playlist's URL.
//
// A single failed search or create doesn't abort the outer loop —
// errors are counted and the import continues. This mirrors the
// hosted backend's behaviour and keeps long imports resilient to
// transient Spotify hiccups.
func importTracks(
	ctx context.Context,
	sp *spotify.Client,
	name string,
	tracks []match.Track,
	out chan<- Event,
) (matched, unmatched, errors int, playlistURL string) {
	pl, err := sp.CreatePlaylist(ctx, name, "Imported from YouTube Music.", false)
	if err != nil {
		emit(ctx, out, Event{
			Type:    EventError,
			Message: fmt.Sprintf("create playlist %q: %v", name, err),
		})
		errors++
		return
	}
	playlistURL = pl.URL

	matchedURIs := make([]string, 0, len(tracks))
	total := len(tracks)
	for i, src := range tracks {
		if ctx.Err() != nil {
			return
		}
		artist := ""
		if len(src.Artists) > 0 {
			artist = src.Artists[0]
		}
		query := match.BuildQuery(src)
		candidates, err := sp.SearchTracks(ctx, query, spotify.SearchLimit)
		if err != nil {
			errors++
			emit(ctx, out, Event{
				Type:            EventTrackUnmatched,
				TrackIndex:      i + 1,
				PlaylistTotal:   total,
				TrackTitle:      src.Title,
				TrackArtist:     artist,
				TrackConfidence: 0,
				TrackReason:     fmt.Sprintf("search failed: %v", err),
			})
			continue
		}
		r := match.Pick(src, candidates)
		if r.Candidate != nil {
			matched++
			matchedURIs = append(matchedURIs, r.Candidate.URI)
			emit(ctx, out, Event{
				Type:            EventTrackMatched,
				TrackIndex:      i + 1,
				PlaylistTotal:   total,
				TrackTitle:      src.Title,
				TrackArtist:     artist,
				TrackConfidence: roundf(r.Confidence, 3),
				TrackURI:        r.Candidate.URI,
			})
			continue
		}
		unmatched++
		reason := "below threshold"
		if len(candidates) == 0 {
			reason = "no candidates"
		}
		emit(ctx, out, Event{
			Type:            EventTrackUnmatched,
			TrackIndex:      i + 1,
			PlaylistTotal:   total,
			TrackTitle:      src.Title,
			TrackArtist:     artist,
			TrackConfidence: roundf(r.Confidence, 3),
			TrackReason:     reason,
		})
	}

	if len(matchedURIs) > 0 {
		if _, err := sp.AddTracks(ctx, pl.ID, matchedURIs); err != nil {
			errors++
			emit(ctx, out, Event{
				Type:    EventError,
				Message: fmt.Sprintf("add tracks to %q: %v", name, err),
			})
		}
	}
	return
}

// filterMusic drops every track whose categoryId != "10" (Music).
// Deleted/private videos have empty category — also dropped.
func filterMusic(tracks []match.Track, cats []string) []match.Track {
	out := make([]match.Track, 0, len(tracks))
	for i, t := range tracks {
		if i < len(cats) && cats[i] == youtube.MusicCategoryID {
			out = append(out, t)
		}
	}
	return out
}

// emit sends an event or gives up if ctx is done — never blocks
// forever on a slow consumer.
func emit(ctx context.Context, out chan<- Event, ev Event) {
	select {
	case out <- ev:
	case <-ctx.Done():
	}
}

func roundf(v float64, places int) float64 {
	// Simple rounding for event emission — we only use 3 decimals
	// for display. Avoids pulling in math.Round to keep import count
	// trivial.
	mul := 1.0
	for i := 0; i < places; i++ {
		mul *= 10
	}
	return float64(int(v*mul+0.5)) / mul
}
