package model

type View int

const (
	ViewHome View = iota
	ViewLibrary
	ViewSearch
	ViewPlaylists
	ViewVisualizer
	ViewLyrics
	ViewSettings
	ViewHelp
	ViewImport
)

type SidebarItem struct {
	View View
	Icon string
	Name string
}

var SidebarItems = []SidebarItem{
	{ViewHome, "⌂ ", "Home"},
	// Labeled with Spotify's own language: this view is the user's saved
	// tracks (GET /me/tracks), which spotify.com presents as "Liked Songs".
	// Users hunting for that "playlist" look here — it is not a playlist in
	// the API and can never appear in the playlists list.
	{ViewLibrary, "♥ ", "Liked Songs"},
	{ViewSearch, "⌕ ", "Search"},
	{ViewPlaylists, "≡ ", "Playlists"},
	{ViewLyrics, "¶ ", "Lyrics"},
	{ViewImport, "⇪ ", "Import"},
	{ViewSettings, "✦ ", "Settings"},
}

type FocusMode int

const (
	FocusSidebar FocusMode = iota
	FocusContent
	FocusRight
)
