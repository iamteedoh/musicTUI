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
	{ViewLibrary, "♪ ", "Library"},
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
