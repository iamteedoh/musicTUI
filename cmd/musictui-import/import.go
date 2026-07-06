package main

import (
	"context"
	"fmt"

	"github.com/iamteedoh/musicTUI/internal/importcore/cliconfig"
	"github.com/iamteedoh/musicTUI/internal/importcore/importer"
	"github.com/iamteedoh/musicTUI/internal/importcore/oauth"
	"github.com/iamteedoh/musicTUI/internal/importcore/services/spotify"
	"github.com/iamteedoh/musicTUI/internal/importcore/services/youtube"
)

func runImport(ctx context.Context, args []string) error {
	fs := flagSet("import")
	source := fs.String("source", "youtube", "library source (youtube)")
	dest := fs.String("dest", "spotify", "destination (spotify)")
	includeLiked := fs.Bool("include-liked", false,
		"include liked videos (off by default — YT Data API can't distinguish liked music from arbitrary liked videos)")
	quiet := fs.Bool("quiet", false, "suppress per-track output (only show playlist summaries)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *source != "youtube" || *dest != "spotify" {
		return fmt.Errorf("first release supports --source=youtube --dest=spotify only")
	}

	cfg, err := cliconfig.Load()
	if err != nil {
		return err
	}
	if !cfg.IsReady() {
		return fmt.Errorf("run `musictui-import setup` first")
	}

	st, err := tokenStore()
	if err != nil {
		return err
	}

	gmgr := &oauth.GoogleTokenManager{
		Store: st,
		Config: oauth.GoogleConfig{
			ClientID:     cfg.Google.ClientID,
			ClientSecret: cfg.Google.ClientSecret,
		},
	}
	smgr := &oauth.SpotifyTokenManager{
		Store: st,
		Config: oauth.SpotifyConfig{
			ClientID:     cfg.Spotify.ClientID,
			ClientSecret: cfg.Spotify.ClientSecret,
		},
	}

	yt := youtube.NewClient(gmgr.AccessToken)
	sp := spotify.NewClient(smgr.AccessToken)

	events := importer.Run(ctx, yt, sp, importer.Request{
		Source:       *source,
		Dest:         *dest,
		IncludeLiked: *includeLiked,
	})

	return render(events, *quiet)
}

// render drains the event channel, printing human-readable progress.
// Keeps per-track output on a single re-written line so the terminal
// doesn't fill up with thousands of lines — unless --quiet is set,
// in which case only playlist-level events print.
func render(events <-chan importer.Event, quiet bool) error {
	var currentPlaylist string
	var currentTotal int
	for ev := range events {
		switch ev.Type {
		case importer.EventJobStarted:
			fmt.Printf("Importing %s → %s\n\n", ev.Source, ev.Dest)
		case importer.EventPlaylistStarted:
			currentPlaylist = ev.PlaylistName
			currentTotal = ev.PlaylistTotal
			extra := ""
			if ev.FilteredNonMusic > 0 {
				extra = fmt.Sprintf(" (filtered %d non-music)", ev.FilteredNonMusic)
			}
			fmt.Printf("▸ %s — %d tracks%s\n", currentPlaylist, currentTotal, extra)
		case importer.EventPlaylistSkipped:
			reason := ev.SkipReason
			if ev.FilteredNonMusic > 0 && reason == "no music tracks (all filtered by category)" {
				reason = fmt.Sprintf("%d non-music tracks", ev.FilteredNonMusic)
			}
			fmt.Printf("○ skipped %q — %s\n", ev.PlaylistName, reason)
		case importer.EventTrackMatched:
			if !quiet {
				fmt.Printf("\r  %d/%d  ✓ %s — %s  (%.2f)            ",
					ev.TrackIndex, currentTotal, truncate(ev.TrackTitle, 40), truncate(ev.TrackArtist, 20), ev.TrackConfidence)
			}
		case importer.EventTrackUnmatched:
			if !quiet {
				fmt.Printf("\r  %d/%d  ✗ %s — %s  (%s)            ",
					ev.TrackIndex, currentTotal, truncate(ev.TrackTitle, 40), truncate(ev.TrackArtist, 20), ev.TrackReason)
			}
		case importer.EventPlaylistDone:
			if !quiet {
				fmt.Print("\r\033[K") // clear current line
			}
			fmt.Printf("  done: %d matched / %d unmatched / %d errors  %s\n\n",
				ev.Matched, ev.Unmatched, ev.Errors, ev.PlaylistURL)
		case importer.EventJobDone:
			fmt.Println("───")
			fmt.Printf("✓ Import complete: %d playlists, %d tracks matched, %d unmatched, %d errors\n",
				ev.PlaylistCount, ev.Matched, ev.Unmatched, ev.Errors)
		case importer.EventError:
			fmt.Printf("⚠ %s\n", ev.Message)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s + spaces(n-len(s))
	}
	return s[:n-1] + "…"
}

func spaces(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}
