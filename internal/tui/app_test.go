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
