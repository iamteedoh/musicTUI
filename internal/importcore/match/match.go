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
	// are easy to game (many songs share one-name artists like "Drake").
	score := 0.7*titleSim + 0.3*artistSim

	// Wrong-artist gate: when BOTH sides carry artist info and there is
	// zero overlap, the candidate is almost certainly a cover, karaoke,
	// or tribute version of the same title — exactly the silent
	// corruption an import must not commit. Cap the score below the
	// accept threshold so the track is reported as unmatched (loud and
	// user-fixable) instead. An exact title alone used to score 0.7 and
	// auto-accept, which put cover versions into imported playlists.
	if artistSim == 0 && len(src.Artists) > 0 && len(cand.Artists) > 0 {
		if capped := MatchThreshold - 0.05; score > capped {
			score = capped
		}
		return score
	}

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
	bnorm := make([]string, 0, len(b))
	for _, n := range b {
		if bn := normalizeArtist(n); bn != "" {
			bnorm = append(bnorm, bn)
		}
	}
	var hit int
	for _, n := range a {
		an := normalizeArtist(n)
		if an == "" {
			continue
		}
		for _, bn := range bnorm {
			// Equal, equal-ignoring-spaces ("AC/DC" vs "ACDC"), or one
			// contains the other as a whole word ("Karol G, Shakira").
			if an == bn || squash(an) == squash(bn) ||
				artistContains(an, bn) || artistContains(bn, an) {
				hit++
				break
			}
		}
	}
	return float64(hit) / float64(len(a))
}

// squash removes spaces for punctuation-insensitive comparison.
func squash(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

// artistContains reports whether outer contains inner as a whole word —
// "karol g shakira" contains "karol g", but "meatloaf" does not contain
// "eat". Guards against trivially short inners.
func artistContains(outer, inner string) bool {
	if len(inner) < 3 || len(inner) >= len(outer) {
		return false
	}
	idx := strings.Index(outer, inner)
	if idx < 0 {
		return false
	}
	beforeOK := idx == 0 || outer[idx-1] == ' '
	end := idx + len(inner)
	afterOK := end == len(outer) || outer[end] == ' '
	return beforeOK && afterOK
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

// accentFolder transliterates common Latin diacritics and ligatures to
// their ASCII base letters so "Bahía"/"Bahia" and "Beyoncé"/"Beyonce"
// normalize identically. The previous behavior DELETED non-ASCII runes
// (Go regexp's \w is ASCII-only), so the accented and unaccented forms of
// the same name never matched — international artists scored 0 overlap.
var accentFolder = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "ā", "a", "ă", "a", "ą", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "ē", "e", "ė", "e", "ę", "e", "ě", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i", "ī", "i", "į", "i", "ı", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o", "ō", "o", "ő", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ū", "u", "ů", "u", "ű", "u", "ų", "u",
	"ý", "y", "ÿ", "y",
	"ñ", "n", "ń", "n", "ň", "n",
	"ç", "c", "ć", "c", "č", "c",
	"š", "s", "ś", "s", "ş", "s", "ș", "s",
	"ž", "z", "ź", "z", "ż", "z",
	"ł", "l", "ľ", "l",
	"ď", "d", "đ", "d",
	"ť", "t", "ţ", "t", "ț", "t",
	"ř", "r",
	"ğ", "g",
	"ß", "ss", "æ", "ae", "œ", "oe", "ð", "d", "þ", "th",
)

func foldAccents(s string) string {
	return accentFolder.Replace(s)
}

func normalizeTitle(s string) string {
	s = foldAccents(strings.ToLower(s))
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

// normalizeArtist lowercases, folds accents to ASCII (so "Beyoncé" and
// "beyonce" really do collapse identically), strips a leading "the "
// ("The Beatles" vs "Beatles"), and removes remaining punctuation while
// preserving word boundaries for whole-word containment checks.
func normalizeArtist(s string) string {
	s = foldAccents(strings.ToLower(s))
	s = strings.TrimPrefix(s, "the ")
	s = punctRun.ReplaceAllString(s, " ")
	s = whitespaceRun.ReplaceAllString(s, " ")
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
