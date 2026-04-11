package model

import (
	"fmt"
	"time"
)

type Artist struct {
	ID       string
	Name     string
	ImageURL string
	URI      string
}

type AlbumRef struct {
	ID       string
	Name     string
	ImageURL string
}

type Track struct {
	ID          string
	Name        string
	Artists     []Artist
	Album       *AlbumRef
	Duration    time.Duration
	TrackNumber uint32
	DiscNumber  uint32
	Explicit    bool
	URI         string
}

func (t Track) ArtistNames() string {
	if len(t.Artists) == 0 {
		return "Unknown Artist"
	}
	s := t.Artists[0].Name
	for _, a := range t.Artists[1:] {
		s += ", " + a.Name
	}
	return s
}

func (t Track) FormatDuration() string {
	m := int(t.Duration.Minutes())
	s := int(t.Duration.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

type Album struct {
	ID          string
	Name        string
	Artists     []Artist
	Tracks      []Track
	ReleaseDate string
	TotalTracks uint32
	ImageURL    string
	URI         string
}

type Playlist struct {
	ID          string
	Name        string
	Description string
	Owner       string
	TrackCount  uint32
	ImageURL    string
	URI         string
}

type Page[T any] struct {
	Items  []T
	Total  uint32
	Offset uint32
	Limit  uint32
}

func (p Page[T]) HasNext() bool {
	return p.Offset+p.Limit < p.Total
}

func (p Page[T]) NextOffset() uint32 {
	return p.Offset + p.Limit
}

type SearchResults struct {
	Tracks      []Track
	Albums      []Album
	Artists     []Artist
	Playlists   []Playlist
	PrimaryType string // "artist", "album", or "track" (default: "track")
}
