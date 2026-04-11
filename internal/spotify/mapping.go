package spotify

import (
	"time"

	spotifylib "github.com/zmb3/spotify/v2"

	"github.com/iamteedoh/musictui-go/internal/model"
)

func mapArtist(a spotifylib.SimpleArtist) model.Artist {
	return model.Artist{
		ID:   string(a.ID),
		Name: a.Name,
		URI:  string(a.URI),
	}
}

func mapFullArtist(a spotifylib.FullArtist) model.Artist {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	return model.Artist{
		ID:       string(a.ID),
		Name:     a.Name,
		ImageURL: img,
		URI:      string(a.URI),
	}
}

func mapAlbumRef(a spotifylib.SimpleAlbum) *model.AlbumRef {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	return &model.AlbumRef{
		ID:       string(a.ID),
		Name:     a.Name,
		ImageURL: img,
	}
}

func mapAlbum(a spotifylib.SimpleAlbum) model.Album {
	img := ""
	if len(a.Images) > 0 {
		img = a.Images[0].URL
	}
	artists := make([]model.Artist, len(a.Artists))
	for i, art := range a.Artists {
		artists[i] = mapArtist(art)
	}
	return model.Album{
		ID:          string(a.ID),
		Name:        a.Name,
		Artists:     artists,
		ReleaseDate: a.ReleaseDate,
		ImageURL:    img,
		URI:         string(a.URI),
	}
}

func mapTrack(t spotifylib.FullTrack) model.Track {
	artists := make([]model.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = mapArtist(a)
	}
	var album *model.AlbumRef
	if t.Album.ID != "" {
		album = mapAlbumRef(t.Album)
	}
	return model.Track{
		ID:          string(t.ID),
		Name:        t.Name,
		Artists:     artists,
		Album:       album,
		Duration:    time.Duration(t.Duration) * time.Millisecond,
		TrackNumber: uint32(t.TrackNumber),
		DiscNumber:  uint32(t.DiscNumber),
		Explicit:    t.Explicit,
		URI:         string(t.URI),
	}
}

func mapSimpleTrack(t spotifylib.SimpleTrack) model.Track {
	artists := make([]model.Artist, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = mapArtist(a)
	}
	return model.Track{
		ID:          string(t.ID),
		Name:        t.Name,
		Artists:     artists,
		Duration:    time.Duration(t.Duration) * time.Millisecond,
		TrackNumber: uint32(t.TrackNumber),
		DiscNumber:  uint32(t.DiscNumber),
		Explicit:    t.Explicit,
		URI:         string(t.URI),
	}
}

func mapPlaylist(p spotifylib.SimplePlaylist) model.Playlist {
	img := ""
	if len(p.Images) > 0 {
		img = p.Images[0].URL
	}
	owner := ""
	if p.Owner.DisplayName != "" {
		owner = p.Owner.DisplayName
	} else {
		owner = string(p.Owner.ID)
	}
	return model.Playlist{
		ID:         string(p.ID),
		Name:       p.Name,
		Owner:      owner,
		TrackCount: uint32(p.Tracks.Total),
		ImageURL:   img,
		URI:        string(p.URI),
	}
}
