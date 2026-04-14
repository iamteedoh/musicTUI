package ytmusic

import (
	"reflect"
	"testing"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]int{
		"3:42":    222,
		"0:30":    30,
		"1:23:45": 5025,
		"10:00":   600,
		"":        0,
		"invalid": 0,
		"1:2:3:4": 0, // too many colons
	}
	for in, want := range cases {
		if got := parseDuration(in); got != want {
			t.Errorf("parseDuration(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseTrackCountSuffix(t *testing.T) {
	cases := map[string]int{
		"42 tracks":          42,
		"Playlist · 7 songs": 7,
		"1 song":             1,
		"no digits here":     0,
		"":                   0,
		"999 items · extra":  999,
	}
	for in, want := range cases {
		if got := parseTrackCountSuffix(in); got != want {
			t.Errorf("parseTrackCountSuffix(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsYear(t *testing.T) {
	if !isYear("2023") {
		t.Error("2023 should be year")
	}
	if isYear("23") || isYear("abcd") || isYear("20233") {
		t.Error("non-year strings matched")
	}
}

func TestWalk(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{
			"b": []any{
				"zero",
				map[string]any{"c": "found"},
			},
		},
	}
	if got := asString(walk(data, "a", "b", 1, "c")); got != "found" {
		t.Errorf("walk returned %q, want found", got)
	}
	// Missing keys / wrong types should return nil without panicking.
	if walk(data, "missing") != nil {
		t.Error("missing key should yield nil")
	}
	if walk(data, "a", "b", 99) != nil {
		t.Error("out-of-range index should yield nil")
	}
	if walk(data, "a", 0) != nil {
		t.Error("int index into map should yield nil")
	}
}

func TestParseArtistsAndAlbum(t *testing.T) {
	runs := []any{
		map[string]any{
			"text": "Artist A",
			"navigationEndpoint": map[string]any{
				"browseEndpoint": map[string]any{"browseId": "UCartistA"},
			},
		},
		map[string]any{"text": " · "},
		map[string]any{
			"text": "Artist B",
			"navigationEndpoint": map[string]any{
				"browseEndpoint": map[string]any{"browseId": "UCartistB"},
			},
		},
		map[string]any{"text": " · "},
		map[string]any{
			"text": "Cool Album",
			"navigationEndpoint": map[string]any{
				"browseEndpoint": map[string]any{"browseId": "MPREb_album123"},
			},
		},
	}
	artists, album := parseArtistsAndAlbum(runs)
	wantArtists := []string{"Artist A", "Artist B"}
	if !reflect.DeepEqual(artists, wantArtists) {
		t.Errorf("artists = %v, want %v", artists, wantArtists)
	}
	if album != "Cool Album" {
		t.Errorf("album = %q, want Cool Album", album)
	}
}

func TestRunsText(t *testing.T) {
	runs := []any{
		map[string]any{"text": "Artist A"},
		map[string]any{"text": " & "},
		map[string]any{"text": "Artist B"},
	}
	if got := runsText(runs); got != "Artist A & Artist B" {
		t.Errorf("runsText = %q", got)
	}
}
