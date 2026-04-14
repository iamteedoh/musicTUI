package ytmusic

import (
	"context"
	"regexp"
	"strings"

	"github.com/iamteedoh/musicTUI/internal/model"
	sp "github.com/iamteedoh/musicTUI/internal/spotify"
)

// Matching a YT Music track to a Spotify track is the hard part of
// the import. YT surfaces raw song metadata (title + artists), Spotify
// search returns up to 20 candidates, and we need to decide if any are
// "the same song". The scoring function here is intentionally simple:
//
//   1. Normalize both strings — strip the long tail of variant
//      annotations ("(Remastered 2011)", "- Live from London", etc.)
//      that cause distinct-but-should-match songs to look different.
//   2. Require the Spotify candidate's normalized title to be either
//      identical or a prefix/suffix of the YT title (or vice versa).
//   3. Require at least one artist name overlap (after normalization).
//   4. Confidence is weighted by title similarity (0.7) and artist
//      overlap ratio (0.3).
//
// Anything below MatchThreshold becomes "unmatched" in the summary —
// surfacing misses loudly so the user can fix them rather than
// silently losing tracks.

const (
	// MatchThreshold is the confidence floor for auto-accepting a match.
	// 0.60 is conservative; tune based on real import data.
	MatchThreshold = 0.60
)

// Match is the per-track outcome of the import.
type Match struct {
	Source       Track        // the YT Music track we searched for
	SpotifyTrack *model.Track // best Spotify candidate; nil if none above threshold
	Confidence   float64      // 0.0 to 1.0; 0 means no acceptable match
	Error        error        // non-nil if the Spotify search itself failed
}

// URI returns the matched Spotify URI, or empty string for unmatched.
func (m Match) URI() string {
	if m.SpotifyTrack == nil {
		return ""
	}
	return m.SpotifyTrack.URI
}

// MatchTrack searches Spotify for one YT Music track and returns the
// best match plus a confidence score. Never errors on "no good match" —
// that's expressed as Confidence == 0 and SpotifyTrack == nil. An
// actual error means the Spotify search call itself failed (network,
// auth) and the caller should probably retry or abort.
func MatchTrack(ctx context.Context, spClient *sp.Client, yt Track) Match {
	if yt.Title == "" {
		return Match{Source: yt}
	}
	query := buildQuery(yt)
	results, _, err := spClient.Search(ctx, query)
	if err != nil {
		return Match{Source: yt, Error: err}
	}

	var best *model.Track
	var bestScore float64
	for i := range results.Tracks {
		cand := &results.Tracks[i]
		score := scoreTrack(yt, *cand)
		if score > bestScore {
			bestScore = score
			best = cand
		}
	}
	if bestScore < MatchThreshold {
		return Match{Source: yt, Confidence: bestScore}
	}
	return Match{Source: yt, SpotifyTrack: best, Confidence: bestScore}
}

// MatchTracks runs MatchTrack sequentially over a slice. Returns a
// parallel slice of Match values in the same order as input. Errors
// from individual searches are captured on each Match, not propagated;
// the caller decides whether to abort.
//
// Sequential instead of concurrent: Spotify's search endpoint has a
// per-client rate limit that's easy to hit with aggressive parallelism,
// and a few hundred tracks still complete in well under a minute. If
// we grow to imports in the thousands, a bounded worker pool with
// exponential backoff on 429s is the right upgrade.
func MatchTracks(ctx context.Context, spClient *sp.Client, tracks []Track, progress func(done, total int)) []Match {
	out := make([]Match, len(tracks))
	for i, t := range tracks {
		if ctx.Err() != nil {
			break
		}
		out[i] = MatchTrack(ctx, spClient, t)
		if progress != nil {
			progress(i+1, len(tracks))
		}
	}
	return out
}

// ─────────────────── query construction ───────────────────

// buildQuery picks a single search string Spotify will likely return
// good candidates for. Empirically, "title artist" beats plain title
// for disambiguation (too many songs share titles) and beats quoted
// exact-phrase search (YT's titles frequently don't match Spotify's
// canonical form word-for-word).
func buildQuery(t Track) string {
	q := normalizeForSearch(t.Title)
	if len(t.Artists) > 0 {
		q += " " + normalizeForSearch(t.Artists[0])
	}
	return strings.TrimSpace(q)
}

// ─────────────────── scoring ───────────────────

// scoreTrack returns a confidence score in [0, 1]. The formula
// combines title similarity (Jaccard token overlap after aggressive
// normalization) and artist overlap (ratio of YT artists that appear
// in the Spotify candidate's artists).
func scoreTrack(yt Track, sp model.Track) float64 {
	titleSim := jaccard(tokens(normalizeTitle(yt.Title)),
		tokens(normalizeTitle(sp.Name)))

	artistSim := artistOverlap(yt.Artists, spArtistNames(sp))

	// Weighted blend. Title carries more weight because artist matches
	// are easy to game (many songs have one-name artists like "Drake"),
	// but a high title match with zero artist overlap is still a red
	// flag — so we don't let artistSim==0 drop confidence below the
	// threshold on its own without a perfect title.
	score := 0.7*titleSim + 0.3*artistSim

	// Bonus: exact normalized title match + any artist overlap → push
	// toward 1.0 so high-confidence matches reliably clear the threshold.
	if titleSim == 1.0 && artistSim > 0 {
		score = 0.95 + 0.05*artistSim
	}
	return score
}

func spArtistNames(t model.Track) []string {
	out := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		out = append(out, a.Name)
	}
	return out
}

func artistOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	bset := make(map[string]struct{}, len(b))
	for _, n := range b {
		bset[normalizeArtist(n)] = struct{}{}
	}
	var hit int
	for _, n := range a {
		if _, ok := bset[normalizeArtist(n)]; ok {
			hit++
		}
	}
	return float64(hit) / float64(len(a))
}

// ─────────────────── normalization ───────────────────

// normalizeTitle strips the common suffixes that show up in one
// service's title but not the other's: "(Remastered)", "- Live at X",
// "(feat. X)", etc. Keeping these in the comparison causes plausible
// matches to miss the threshold for no real reason.
var (
	parenTail      = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
	// (?i): case-insensitive. normalizeTitle lowercases before running
	// this, but normalizeForSearch (used for building Spotify query)
	// runs on the original case, so leaving both casings covered here
	// avoids a second almost-identical regex.
	dashTail       = regexp.MustCompile(`(?i)\s*-\s+(remaster(ed)?|live|mono|stereo|radio edit|single version|album version|extended version|acoustic|demo|instrumental)\b.*$`)
	featParen      = regexp.MustCompile(`\s*\(feat\.?\s[^)]*\)\s*`)
	featBrackets   = regexp.MustCompile(`\s*\[feat\.?\s[^\]]*\]\s*`)
	punctRun       = regexp.MustCompile(`[^\w\s]+`)
	whitespaceRun  = regexp.MustCompile(`\s+`)
)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Strip "(feat. ...)" variations regardless of position — always noise for matching.
	s = featParen.ReplaceAllString(s, " ")
	s = featBrackets.ReplaceAllString(s, " ")
	// Remove trailing "(annotation)" of any kind.
	for parenTail.MatchString(s) {
		s = parenTail.ReplaceAllString(s, "")
	}
	// Remove "- Remastered 2011" / "- Live at X" style suffixes.
	s = dashTail.ReplaceAllString(s, "")
	// Collapse punctuation and whitespace so "Don't" vs "Dont" match.
	s = punctRun.ReplaceAllString(s, " ")
	s = whitespaceRun.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// normalizeArtist is simpler — artist names are less cluttered than
// track titles. Just lowercase and strip non-alphanumerics so
// "Beyoncé" and "beyonce" are considered identical.
func normalizeArtist(s string) string {
	s = strings.ToLower(s)
	s = punctRun.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// normalizeForSearch is what we give to Spotify's search box. We
// keep this more forgiving than normalizeTitle so Spotify can still
// full-text-match against its canonical metadata.
func normalizeForSearch(s string) string {
	s = featParen.ReplaceAllString(s, " ")
	s = featBrackets.ReplaceAllString(s, " ")
	for parenTail.MatchString(s) {
		s = parenTail.ReplaceAllString(s, "")
	}
	s = dashTail.ReplaceAllString(s, "")
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

// tokens splits on whitespace and discards empty tokens.
func tokens(s string) []string {
	f := strings.Fields(s)
	return f
}

// jaccard is the Jaccard similarity of two token multisets: the size
// of their intersection over the size of their union. 1.0 means
// identical token sets; 0.0 means completely disjoint.
func jaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	aset := make(map[string]struct{}, len(a))
	for _, t := range a {
		aset[t] = struct{}{}
	}
	bset := make(map[string]struct{}, len(b))
	for _, t := range b {
		bset[t] = struct{}{}
	}
	var inter int
	for t := range aset {
		if _, ok := bset[t]; ok {
			inter++
		}
	}
	union := len(aset) + len(bset) - inter
	return float64(inter) / float64(union)
}
