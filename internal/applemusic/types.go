// Package applemusic implements a client for Apple Music's public
// user library API, used for importing a user's playlists / saved
// albums / liked songs into Spotify.
//
// Unlike YouTube Music where we reverse-engineer the internal
// InnerTube endpoints, Apple Music exposes an official documented
// Web API at api.music.apple.com. The catch: authentication requires
// a Developer Token (which needs an Apple Developer Program
// membership, $99/yr) AND a Music User Token (which can only be
// minted via MusicKit — the browser-based JS SDK, the iOS native
// framework, or the macOS native framework). Go has access to none
// of these.
//
// Workaround: we ship a tiny static HTML page (see page/apple-auth/)
// that uses MusicKit JS to sign the user in and POST their Music User
// Token back to a local callback URL this package listens on — the
// same pattern Spotify OAuth uses. The page is hosted on the user's
// own doralab for now; can move to a public static host later.
package applemusic

// Track in the user's Apple Music library. Canonical fields for
// Spotify matching — we don't try to preserve Apple-specific metadata
// like isrc or explicit here because Spotify search does better with
// textual title + artist inputs than with structured metadata.
type Track struct {
	ID       string // Apple Music track ID ("i.XXX" for library items, "15XXXXXX" for catalog)
	Title    string
	Artists  []string
	Album    string
	Duration int // seconds
}

// Playlist in the user's Apple Music library.
type Playlist struct {
	ID         string // "p.XXX" for user-created, "pl.XXX" for Apple-curated
	Name       string
	TrackCount int
}

// Album in the user's Apple Music library (saved albums).
type Album struct {
	ID      string
	Name    string
	Artists []string
	Year    string
}

// Artist the user has in their library (favorites).
type Artist struct {
	ID   string
	Name string
}
