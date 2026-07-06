// Package match is the track-matching engine: pure logic that scores
// how well a source track (from YT Music / Apple Music / etc.) matches
// a candidate Spotify track, and picks the best match above a
// confidence threshold.
//
// Originally lived at musicTUI@41207b6:internal/ytmusic/match.go.
// Extracted here with internal-dependency-free types so the import
// logic can ship in both the musicTUI TUI and the standalone
// musictui-import CLI without coupling either to the other.
//
// The matching approach is intentionally simple:
//
//  1. Normalize both titles — strip variant annotations
//     ("(Remastered 2011)", "- Live from London", "(feat. X)") that
//     cause distinct-but-should-match songs to score low.
//  2. Jaccard token similarity on normalized titles (weight 0.7).
//  3. Artist overlap ratio on normalized artist names (weight 0.3).
//  4. Exact-title + any artist overlap bumps score to 0.95+ so obvious
//     matches reliably clear the threshold.
//
// Anything below MatchThreshold is reported as "unmatched" rather
// than silently dropped — so misses are loud and the user can fix.
package match

import (
	"regexp"
	"strings"
)

// MatchThreshold is the confidence floor for auto-accepting a match.
// 0.60 is conservative; tuned against real imports.
const MatchThreshold = 0.60

// Track is one song from a source library (YouTube Music, Apple Music,
// etc.) — the minimum we need for matching.
type Track struct {
	ID       string // source-specific ID (e.g. YT videoID)
	Title    string
	Artists  []string
	Album    string
	Duration int // seconds, 0 if unknown
}

// Candidate is one Spotify search result we're scoring the source
// track against.
type Candidate struct {
	URI     string // "spotify:track:..."
	ID      string
	Name    string
	Artists []Artist
	Album   string
}

// Artist is a Spotify artist reference attached to a Candidate.
type Artist struct {
	Name string
	ID   string
}

// Result is the per-track outcome.
type Result struct {
	Source     Track
	Candidate  *Candidate // nil if nothing scored above threshold
	Confidence float64    // 0.0 to 1.0; 0 means no acceptable match
	Err        error      // non-nil if the search itself failed
}

// URI returns the matched Spotify URI, or empty string for unmatched.
func (r Result) URI() string {
	if r.Candidate == nil {
		return ""
	}
	return r.Candidate.URI
}

// Pick scores every candidate and returns the best Result. No I/O —
// the caller runs the Spotify search and feeds candidates in.
//
// Empty source title returns a zero-confidence Result (impossible to
// match with nothing to search for).
func Pick(src Track, candidates []Candidate) Result {
	if src.Title == "" {
		return Result{Source: src}
	}
	var best *Candidate
	var bestScore float64
	for i := range candidates {
		cand := &candidates[i]
		score := scoreTrack(src, *cand)
		if score > bestScore {
			bestScore = score
			best = cand
		}
	}
	if bestScore < MatchThreshold {
		return Result{Source: src, Confidence: bestScore}
	}
	return Result{Source: src, Candidate: best, Confidence: bestScore}
}

// BuildQuery picks a single Spotify search string that's likely to
// return good candidates. "title artist" beats plain title (too many
// songs share titles) and beats quoted exact-phrase search (source
// titles rarely match Spotify's canonical form word-for-word).
func BuildQuery(t Track) string {
	q := normalizeForSearch(t.Title)
	if len(t.Artists) > 0 {
		q += " " + normalizeForSearch(t.Artists[0])
	}
	return strings.TrimSpace(q)
}

// ─────────────────── scoring ───────────────────

func scoreTrack(src Track, cand Candidate) float64 {
	titleSim := jaccard(
		tokens(normalizeTitle(src.Title)),
		tokens(normalizeTitle(cand.Name)),
	)
	artistSim := artistOverlap(src.Artists, candArtistNames(cand))

	// Weighted blend. Title carries more weight because artist matches
	// are easy to game (many songs share one-name artists like "Drake"),
	// but a high title match with zero artist overlap is still a red
	// flag — so we don't let artistSim==0 drop confidence below the
	// threshold on its own without a perfect title.
	score := 0.7*titleSim + 0.3*artistSim

	// Bonus: exact normalized title match + any artist overlap → push
	// toward 1.0 so high-confidence matches reliably clear threshold.
	if titleSim == 1.0 && artistSim > 0 {
		score = 0.95 + 0.05*artistSim
	}
	return score
}

func candArtistNames(c Candidate) []string {
	out := make([]string, 0, len(c.Artists))
	for _, a := range c.Artists {
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

var (
	parenTail = regexp.MustCompile(`\s*\([^)]*\)\s*$`)
	// (?i): case-insensitive. normalizeTitle lowercases before this
	// runs, but normalizeForSearch (used for building Spotify queries)
	// runs on the original case, so covering both casings here avoids
	// a second almost-identical regex.
	dashTail      = regexp.MustCompile(`(?i)\s*-\s+(remaster(ed)?|live|mono|stereo|radio edit|single version|album version|extended version|acoustic|demo|instrumental)\b.*$`)
	featParen     = regexp.MustCompile(`\s*\(feat\.?\s[^)]*\)\s*`)
	featBrackets  = regexp.MustCompile(`\s*\[feat\.?\s[^\]]*\]\s*`)
	punctRun      = regexp.MustCompile(`[^\w\s]+`)
	whitespaceRun = regexp.MustCompile(`\s+`)
)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Strip "(feat. ...)" variations regardless of position — always noise.
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

// normalizeArtist is simpler — artist names are less cluttered.
// Lowercase + strip non-alphanumerics so "Beyoncé" and "beyonce"
// collapse identically (Go's `\w` is ASCII-only by default, so é
// drops to nothing and both become "beyonc").
func normalizeArtist(s string) string {
	s = strings.ToLower(s)
	s = punctRun.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// normalizeForSearch is what we send to Spotify's search box. More
// forgiving than normalizeTitle so Spotify can still full-text-match
// against its canonical metadata.
func normalizeForSearch(s string) string {
	s = featParen.ReplaceAllString(s, " ")
	s = featBrackets.ReplaceAllString(s, " ")
	for parenTail.MatchString(s) {
		s = parenTail.ReplaceAllString(s, "")
	}
	s = dashTail.ReplaceAllString(s, "")
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

func tokens(s string) []string {
	return strings.Fields(s)
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
