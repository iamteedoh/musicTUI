package match

import "testing"

func TestNormalizeTitle(t *testing.T) {
	cases := map[string]string{
		"Bohemian Rhapsody - Remastered 2011":   "bohemian rhapsody",
		"Bohemian Rhapsody (Remastered 2011)":   "bohemian rhapsody",
		"Old Town Road (feat. Billy Ray Cyrus)": "old town road",
		"Old Town Road [feat. Billy Ray Cyrus]": "old town road",
		"Don't Stop Me Now":                     "don t stop me now",
		"HUMBLE.":                               "humble",
		"Hotel California - Live at The Forum":  "hotel california",
		"Thriller":                              "thriller",
	}
	for in, want := range cases {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJaccard(t *testing.T) {
	cases := []struct {
		a, b []string
		want float64
	}{
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, 1.0},
		{[]string{"a", "b"}, []string{"a", "b", "c"}, 2.0 / 3.0},
		{[]string{"a"}, []string{"b"}, 0.0},
		{[]string{}, []string{"a"}, 0.0},
		{[]string{"a", "b"}, []string{"c", "d"}, 0.0},
	}
	for _, c := range cases {
		if got := jaccard(c.a, c.b); got != c.want {
			t.Errorf("jaccard(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestScoreTrack_IdenticalMatch(t *testing.T) {
	src := Track{Title: "Bohemian Rhapsody", Artists: []string{"Queen"}}
	cand := Candidate{Name: "Bohemian Rhapsody", Artists: []Artist{{Name: "Queen"}}}
	if got := scoreTrack(src, cand); got < 0.95 {
		t.Errorf("identical match scored %v, want ≥0.95", got)
	}
}

func TestScoreTrack_RemasterSuffixIgnored(t *testing.T) {
	src := Track{Title: "Bohemian Rhapsody", Artists: []string{"Queen"}}
	cand := Candidate{Name: "Bohemian Rhapsody - Remastered 2011", Artists: []Artist{{Name: "Queen"}}}
	if got := scoreTrack(src, cand); got < MatchThreshold {
		t.Errorf("remaster should still match, scored %v (threshold %v)", got, MatchThreshold)
	}
}

func TestScoreTrack_FeatRemoved(t *testing.T) {
	src := Track{Title: "Old Town Road (feat. Billy Ray Cyrus)", Artists: []string{"Lil Nas X"}}
	cand := Candidate{
		Name:    "Old Town Road",
		Artists: []Artist{{Name: "Lil Nas X"}, {Name: "Billy Ray Cyrus"}},
	}
	if got := scoreTrack(src, cand); got < MatchThreshold {
		t.Errorf("feat parenthetical should not block match, scored %v", got)
	}
}

func TestScoreTrack_WrongArtistLowerScore(t *testing.T) {
	src := Track{Title: "Thriller", Artists: []string{"Michael Jackson"}}
	cand := Candidate{Name: "Thriller", Artists: []Artist{{Name: "Some Cover Band"}}}
	if got := scoreTrack(src, cand); got >= 0.9 {
		t.Errorf("same title / wrong artist should NOT be high-confidence; got %v", got)
	}
}

func TestPick_Threshold(t *testing.T) {
	src := Track{Title: "Halo", Artists: []string{"Beyonce"}}
	candidates := []Candidate{
		{Name: "Halo", Artists: []Artist{{Name: "Beyonce"}}, URI: "spotify:track:halo"},
		{Name: "Unrelated Song", Artists: []Artist{{Name: "Someone Else"}}, URI: "spotify:track:nope"},
	}
	r := Pick(src, candidates)
	if r.Candidate == nil {
		t.Fatalf("expected a match, got nil")
	}
	if r.Candidate.Name != "Halo" {
		t.Errorf("wrong candidate picked: %s", r.Candidate.Name)
	}
	if r.Confidence < 0.95 {
		t.Errorf("confidence too low: %v", r.Confidence)
	}
}

func TestPick_EmptyTitle(t *testing.T) {
	r := Pick(Track{Title: ""}, []Candidate{{Name: "Whatever"}})
	if r.Candidate != nil {
		t.Errorf("empty title should not match anything")
	}
	if r.Confidence != 0 {
		t.Errorf("empty title should have 0 confidence, got %v", r.Confidence)
	}
}

func TestPick_BelowThreshold(t *testing.T) {
	src := Track{Title: "Very Specific Song Title", Artists: []string{"Some Artist"}}
	candidates := []Candidate{
		{Name: "Completely Different", Artists: []Artist{{Name: "Other Artist"}}},
	}
	r := Pick(src, candidates)
	if r.Candidate != nil {
		t.Errorf("no match should produce nil Candidate")
	}
	if r.Confidence >= MatchThreshold {
		t.Errorf("confidence %v should be below threshold %v", r.Confidence, MatchThreshold)
	}
}

func TestResultURI_Empty(t *testing.T) {
	r := Result{Source: Track{Title: "x"}}
	if r.URI() != "" {
		t.Errorf("nil-candidate URI should be empty, got %q", r.URI())
	}
}

func TestResultURI_Populated(t *testing.T) {
	r := Result{
		Source:     Track{Title: "x"},
		Candidate:  &Candidate{URI: "spotify:track:abc"},
		Confidence: 0.9,
	}
	if r.URI() != "spotify:track:abc" {
		t.Errorf("URI got %q", r.URI())
	}
}

func TestBuildQuery(t *testing.T) {
	cases := []struct {
		track Track
		want  string
	}{
		{Track{Title: "Bohemian Rhapsody", Artists: []string{"Queen"}}, "Bohemian Rhapsody Queen"},
		{Track{Title: "Bohemian Rhapsody (Remastered)", Artists: []string{"Queen"}}, "Bohemian Rhapsody Queen"},
		{Track{Title: "Title Only"}, "Title Only"},
		{Track{Title: ""}, ""},
	}
	for _, c := range cases {
		if got := BuildQuery(c.track); got != c.want {
			t.Errorf("BuildQuery(%+v) = %q, want %q", c.track, got, c.want)
		}
	}
}

// The reported MUS-18 bug: an exact-title candidate by a COMPLETELY
// different artist (cover / karaoke / tribute version) must be rejected,
// not silently imported. It used to score 0.7 (title alone) and pass the
// 0.60 threshold.
func TestPickRejectsWrongArtistCover(t *testing.T) {
	src := Track{Title: "Blinding Lights", Artists: []string{"The Weeknd"}}
	cover := Candidate{
		URI:     "spotify:track:cover",
		Name:    "Blinding Lights",
		Artists: []Artist{{Name: "Karaoke Hits Band"}},
	}
	res := Pick(src, []Candidate{cover})
	if res.Candidate != nil {
		t.Fatalf("wrong-artist cover was accepted (confidence %v) — this corrupts imports", res.Confidence)
	}
}

// With both the original and a cover in the candidate set, the original
// must win even if the cover is listed first.
func TestPickPrefersOriginalOverCover(t *testing.T) {
	src := Track{Title: "Blinding Lights", Artists: []string{"The Weeknd"}}
	cands := []Candidate{
		{URI: "spotify:track:cover", Name: "Blinding Lights", Artists: []Artist{{Name: "Tribute Stars"}}},
		{URI: "spotify:track:orig", Name: "Blinding Lights", Artists: []Artist{{Name: "The Weeknd"}}},
	}
	res := Pick(src, cands)
	if res.Candidate == nil || res.Candidate.URI != "spotify:track:orig" {
		t.Fatalf("expected the original to win, got %+v", res.Candidate)
	}
}

// Accented and unaccented forms of the same artist must match: the old
// normalization DELETED non-ASCII runes, so "Bahía" ("baha") never matched
// "Bahia" ("bahia") and international artists scored zero overlap.
func TestArtistOverlapFoldsAccents(t *testing.T) {
	cases := []struct{ a, b string }{
		{"Mike Bahía", "Mike Bahia"},
		{"Beyoncé", "Beyonce"},
		{"Céline Dion", "Celine Dion"},
		{"Motörhead", "Motorhead"},
	}
	for _, c := range cases {
		if got := artistOverlap([]string{c.a}, []string{c.b}); got != 1.0 {
			t.Errorf("artistOverlap(%q, %q) = %v, want 1.0", c.a, c.b, got)
		}
	}
}

// Leniency: "The X" vs "X", joined artist strings, and punctuation forms.
func TestArtistOverlapLeniency(t *testing.T) {
	cases := []struct{ a, b string }{
		{"The Beatles", "Beatles"},
		{"KAROL G", "KAROL G, Shakira"},
		{"AC/DC", "ACDC"},
	}
	for _, c := range cases {
		if got := artistOverlap([]string{c.a}, []string{c.b}); got != 1.0 {
			t.Errorf("artistOverlap(%q, %q) = %v, want 1.0", c.a, c.b, got)
		}
	}
	// Whole-word containment must not false-positive on substrings.
	if got := artistOverlap([]string{"Eat"}, []string{"Meatloaf"}); got != 0 {
		t.Errorf("artistOverlap(Eat, Meatloaf) = %v, want 0", got)
	}
}

// A source with NO artist info can still match on title alone — the
// wrong-artist gate only applies when both sides carry artists.
func TestPickTitleOnlyWhenSourceHasNoArtist(t *testing.T) {
	src := Track{Title: "Bohemian Rhapsody"}
	cand := Candidate{URI: "spotify:track:x", Name: "Bohemian Rhapsody", Artists: []Artist{{Name: "Queen"}}}
	res := Pick(src, []Candidate{cand})
	if res.Candidate == nil {
		t.Fatalf("artist-less source failed to match on exact title (confidence %v)", res.Confidence)
	}
}
