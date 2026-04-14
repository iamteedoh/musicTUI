package ytmusic

// Canonical import types. These are what the rest of musicTUI consumes
// when asked to read a user's YT Music library — they strip the deeply
// nested YouTube "renderer" JSON down to just the fields a Spotify
// import actually needs.
//
// Keep these minimal on purpose. If a future feature needs more (e.g.
// cover art, track duration), widen here rather than leaking raw
// YouTube JSON into other packages.

// Playlist is a user-library playlist (playlistId + name + track count).
// Track data is fetched separately via GetPlaylistTracks.
type Playlist struct {
	ID         string // YT Music playlist ID (starts with "VL" for public, "PL" otherwise)
	Name       string
	TrackCount int    // 0 if YT didn't include it
	Thumbnail  string // URL, optional
}

// Track is a single song. VideoID is YT Music's identifier (11 chars
// like a YouTube video); it doubles as the play URL basis. For the
// Spotify import, we only use Title + Artists to run a search.
type Track struct {
	VideoID  string
	Title    string
	Artists  []string
	Album    string // optional
	Duration int    // seconds, 0 if unknown
}

// Album in the user's library. TrackCount is often present on the
// library tile and worth keeping for match-confidence signals.
type Album struct {
	BrowseID   string // YT Music browse ID (not same as playlist ID)
	Name       string
	Artists    []string
	Year       string // string because YT sometimes gives "Album · 2019"
	TrackCount int
}

// Artist the user follows. Used during import to re-create a "Followed
// Artists" set on Spotify (via FollowArtist API if scoped, otherwise a
// list in the summary report).
type Artist struct {
	ChannelID string // YT channel ID
	Name      string
}
