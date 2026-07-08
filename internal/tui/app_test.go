package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/importbackend"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
)

// A playlist re-fetch (which always restarts at offset 0, e.g. after a
// stale-token re-auth) must REPLACE the in-memory list, not append a second
// copy of every playlist. The append-only bug doubled the list and tripped a
// false "duplicate playlists" prompt mid-playback — MUS-13.
func TestPlaylistsRefetchReplacesNotAppends(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	pls := []model.Playlist{{Name: "Alpha"}, {Name: "Beta"}, {Name: "Gamma"}}

	m, _ := app.Update(PlaylistsLoadedMsg{Playlists: pls, Total: uint32(len(pls)), Offset: 0})
	app = m.(App)
	if got := len(app.playlist.Items); got != 3 {
		t.Fatalf("after first load: %d playlists, want 3", got)
	}

	// Simulate the background re-fetch that follows a re-auth.
	m, _ = app.Update(PlaylistsLoadedMsg{Playlists: pls, Total: uint32(len(pls)), Offset: 0})
	app = m.(App)
	if got := len(app.playlist.Items); got != 3 {
		t.Fatalf("after re-fetch: %d playlists, want 3 (regression: the list doubled)", got)
	}
}

// A genuinely paginated load (offset > 0 on later pages) must still accumulate.
func TestPlaylistsPaginationStillAccumulates(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	page1 := []model.Playlist{{Name: "A"}, {Name: "B"}}
	page2 := []model.Playlist{{Name: "C"}, {Name: "D"}}

	m, _ := app.Update(PlaylistsLoadedMsg{Playlists: page1, Total: 4, Offset: 0})
	app = m.(App)
	m, _ = app.Update(PlaylistsLoadedMsg{Playlists: page2, Total: 4, Offset: 2})
	app = m.(App)
	if got := len(app.playlist.Items); got != 4 {
		t.Fatalf("after two pages: %d playlists, want 4", got)
	}
}

// Pressing r on the Import error screen must trigger the service reconnect
// (MUS-12's recovery path), not silently cycle the playback repeat mode.
// The global playback-keys switch used to swallow r before the Import view
// ever saw it, so "r: reconnect YouTube" did nothing.
func TestImportErrorScreenReconnectKey(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close() // empty config auto-opens the wizard, which captures keys
	app.view = model.ViewImport
	app.importClient = &importbackend.Client{}
	app.importv.Stage = components.ImportStageError
	app.importv.Err = fmt.Errorf(`youtube token: google refresh: google token: 400: {"error": "invalid_grant"}`)
	repeatBefore := app.queue.Repeat

	m, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app = m.(App)

	if app.queue.Repeat != repeatBefore {
		t.Fatal("r on the Import error screen cycled repeat mode — the playback switch swallowed it again")
	}
	if app.importv.Stage != components.ImportStageAwaitingAuth {
		t.Fatalf("r did not enter the reconnect flow: stage = %v", app.importv.Stage)
	}
	if cmd == nil {
		t.Fatal("r did not return a reauth command")
	}
}

// On Windows, bubbletea reads the console directly and emits a KeyMsg for every
// key-down except Shift. Keys that carry no character (a bare Ctrl, Alt,
// CapsLock or Win press) fall through its keyType() to KeyRunes with a zero
// Char, so they arrive as Runes: []rune{0}. Those NULs used to be inserted
// verbatim into the onboarding field: pressing Ctrl to paste a Client ID
// prefixed it with U+0000, which survived strings.TrimSpace, so Spotify
// answered "Invalid client id" — MUS-23.
func TestOnboardClientIDIgnoresWindowsModifierKeyRunes(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.StartAtClientID("")

	// What a Windows console sends for: Ctrl(+V paste of "a1"), then Alt, "b2".
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{0}}, // bare Ctrl
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
		{Type: tea.KeyRunes, Runes: []rune{'1'}},
		{Type: tea.KeyRunes, Runes: []rune{0}, Alt: true}, // bare Alt
		{Type: tea.KeyRunes, Runes: []rune{'b'}},
		{Type: tea.KeyRunes, Runes: []rune{'2'}},
	}
	for _, k := range keys {
		m, _ := app.handleOnboardKey(k)
		app = m.(App)
	}

	if got := app.onboard.ClientID(); got != "a1b2" {
		t.Fatalf("ClientID() = %q, want %q — a modifier key-down leaked into the field", got, "a1b2")
	}
	if !app.onboard.OnFinalStep() {
		t.Fatal("a modifier key-down navigated away from the Client ID step")
	}
}

// 'h' is vim-style "back" on the wizard's informational steps, but the final
// step is a text field — there it must type, not navigate.
func TestOnboardFinalStepTypesHInsteadOfNavigatingBack(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.StartAtClientID("")

	m, _ := app.handleOnboardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	app = m.(App)

	if !app.onboard.OnFinalStep() {
		t.Fatal("typing 'h' in the Client ID field jumped back a step")
	}
	if got := app.onboard.ClientID(); got != "h" {
		t.Fatalf("ClientID() = %q, want %q", got, "h")
	}

	// Earlier steps keep the vim binding.
	app.onboard.Start()
	app.onboard.Next()
	m, _ = app.handleOnboardKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	app = m.(App)
	if app.onboard.Step != 0 {
		t.Fatalf("'h' on step 1 did not navigate back: step = %d", app.onboard.Step)
	}
}
