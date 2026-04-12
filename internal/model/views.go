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
)

type SidebarItem struct {
	View View
	Icon string
	Name string
}

var SidebarItems = []SidebarItem{
	{ViewHome, "⌂ ", "Home"},
	{ViewLibrary, "♪ ", "Library"},
	{ViewSearch, "⌕ ", "Search"},
	{ViewPlaylists, "≡ ", "Playlists"},
	{ViewLyrics, "¶ ", "Lyrics"},
	{ViewSettings, "✦ ", "Settings"},
}

type FocusMode int

const (
	FocusSidebar FocusMode = iota
	FocusContent
	FocusRight
)
