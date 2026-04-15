package importbackend

// PlaylistSummary mirrors the shape the UI needs from the shared
// module's youtube.Playlist. Keeping it separate so the UI doesn't
// depend on the service-package internals.
type PlaylistSummary struct {
	ID         string
	Name       string
	TrackCount int
}

// YouTubeLibrary is what LoadLibrary returns.
type YouTubeLibrary struct {
	Playlists  []PlaylistSummary
	LikedCount int
}

// ImportRequest mirrors importer.Request but lives here so the UI
// layer only depends on this package, not the shared module.
type ImportRequest struct {
	Source       string
	Dest         string
	PlaylistIDs  []string
	IncludeLiked bool
}
