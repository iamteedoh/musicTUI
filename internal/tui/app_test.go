package tui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/importbackend"
	"github.com/iamteedoh/musicTUI/internal/model"
	"github.com/iamteedoh/musicTUI/internal/theme"
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

// stripANSI removes SGR/CSI escape sequences so a rendered frame can be
// measured in terminal cells.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// A sixel image is painted at absolute cursor coordinates, so the origin the
// layout hands to the artwork must be exactly where the ARTWORK section's
// content begins. Get it wrong and the cover lands on another panel — with no
// visible symptom in any other test. Pin the hand-derived formula in app.go
// against a real rendered frame (MUS-29).
func TestArtworkOriginMatchesRenderedFrame(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close() // an empty config auto-opens the wizard, which owns the screen
	app.width, app.height = 160, 48

	frame := app.View()
	if frame == "" {
		t.Fatal("empty frame")
	}
	col, row := app.artwork.Origin()
	if col <= 0 || row <= 0 {
		t.Fatalf("layout never set an origin: (%d,%d)", col, row)
	}

	lines := strings.Split(frame, "\n")
	dividerIdx := -1
	for i, ln := range lines {
		if strings.Contains(stripANSI(ln), "ARTWORK") {
			dividerIdx = i
			break
		}
	}
	if dividerIdx < 0 {
		t.Fatal("no ARTWORK divider in the rendered frame")
	}

	// Content starts on the line after the divider. Lines are 0-based; screen
	// rows are 1-based.
	if wantRow := dividerIdx + 2; row != wantRow {
		t.Errorf("origin row = %d, but ARTWORK content starts at screen row %d", row, wantRow)
	}

	// The divider line opens with '├' at the right column's left border; the
	// content column is the one after it.
	divider := stripANSI(lines[dividerIdx])
	borderIdx := strings.Index(divider, "├")
	if borderIdx < 0 {
		t.Fatal("ARTWORK divider has no '├' border character")
	}
	wantCol := len([]rune(divider[:borderIdx])) + 2
	if col != wantCol {
		t.Errorf("origin col = %d, but the right column's content starts at screen col %d", col, wantCol)
	}
}

// fakeSixelArtwork puts the artwork into the sixel style with a loaded cover, so
// View publishes a draw that repaintSixelIfClobbered can act on.
func fakeSixelArtwork(app *App) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 200, G: 40, B: 90, A: 255})
		}
	}
	app.artwork.SetStyle(components.StyleSixel)
	app.artwork.SetCellSize(10, 20)
	app.artwork.LoadURL("https://example.invalid/cover.jpg")
	app.artwork.SetFullImage(img, "https://example.invalid/cover.jpg")
	app.artwork.SetAlbumInfo("Album", "Artist")
}

// Bubble Tea rewrites a whole line when ANY column on it changes, and
// JoinHorizontal merges the three columns into one line. So a changing center
// column erases the sixel pixels sharing that line — the cover appears sliced in
// half. The image must be repainted whenever a row it occupies is rewritten, and
// must NOT be repainted when nothing on those rows changed, or the whole payload
// would go to the terminal 60x/second (MUS-29).
func TestSixelRepaintsOnlyWhenItsRowsChange(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close()

	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	app.SetOutput(NewTermWriter(f))

	fakeSixelArtwork(&app)
	app.artwork.SetOrigin(61, 8)
	_ = app.artwork.View(theme.Nord(), 30, 18) // publishes the draw
	seq, row, rows := app.artwork.SixelDraw()
	if seq == "" || rows <= 0 {
		t.Fatal("artwork did not publish a sixel draw")
	}

	// A frame tall enough to contain the cover, every line distinct.
	frame := func(mut func([]string)) string {
		lines := make([]string, row+rows+4)
		for i := range lines {
			lines[i] = fmt.Sprintf("line-%02d", i)
		}
		if mut != nil {
			mut(lines)
		}
		return strings.Join(lines, "\n")
	}

	// Each frame write flushes at most one staged payload; count bytes to see
	// whether one was staged.
	painted := func() bool {
		before, _ := f.Seek(0, io.SeekCurrent)
		_, _ = app.out.Write([]byte("<FRAME>"))
		after, _ := f.Seek(0, io.SeekCurrent)
		return after-before > int64(len("<FRAME>"))
	}

	app.repaintSixelIfClobbered(frame(nil))
	if !painted() {
		t.Fatal("cover was never painted on the first frame")
	}

	app.repaintSixelIfClobbered(frame(nil))
	if painted() {
		t.Fatal("cover repainted even though its rows did not change")
	}

	// A change on a line the cover does not occupy.
	app.repaintSixelIfClobbered(frame(func(l []string) { l[0] = "TITLE CHANGED" }))
	if painted() {
		t.Fatal("a change outside the cover's rows forced a repaint")
	}

	// A change on a line the cover sits on: the pixels there were just erased.
	app.repaintSixelIfClobbered(frame(func(l []string) {
		l[0] = "TITLE CHANGED"
		l[row-1+rows/2] = "CENTER PANEL CHANGED"
	}))
	if !painted() {
		t.Fatal("cover was not repainted after its rows were rewritten")
	}

	// A modal covers the artwork: never paint over it, and forget the rows so
	// dismissing it (which restores them byte for byte) still repaints.
	app.modal.Active = true
	app.repaintSixelIfClobbered(frame(nil))
	if painted() {
		t.Fatal("painted the cover on top of a modal")
	}
	app.modal.Active = false
	app.repaintSixelIfClobbered(frame(nil))
	if !painted() {
		t.Fatal("cover not repainted after the modal was dismissed")
	}
}

// Resizing moves the artwork panel, so the cover is repainted at new
// coordinates. Konsole does not erase sixel pixels when text is written over
// them, so the old copy stays where it was and bleeds through the tracklist.
// Only an explicit screen erase clears it — and only sixel needs that, so
// character-art terminals must not eat a clear-and-full-repaint on every drag
// of the window edge (MUS-29).
func TestResizeClearsScreenOnlyForSixel(t *testing.T) {
	cases := []struct {
		name      string
		style     components.ArtworkStyle
		wantClear bool
	}{
		{"sixel needs the erase", components.StyleSixel, true},
		{"kitty binds pixels to cells", components.StyleKitty, false},
		{"blocks are just text", components.StyleBlocks, false},
		{"braille is just text", components.StyleBraille, false},
	}
	for _, c := range cases {
		app := NewApp(config.Config{}, "", "test")
		app.onboard.Close()
		app.artwork.SetStyle(c.style)

		m, cmd := app.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
		app = m.(App)

		if app.width != 160 || app.height != 48 {
			t.Fatalf("%s: resize was not recorded", c.name)
		}
		if app.cache.artRows != "" {
			t.Errorf("%s: cached artwork rows survived a resize", c.name)
		}

		gotClear := false
		if cmd != nil {
			gotClear = fmt.Sprintf("%T", cmd()) == "tea.clearScreenMsg"
		}
		if gotClear != c.wantClear {
			t.Errorf("%s: clear-screen on resize = %v, want %v", c.name, gotClear, c.wantClear)
		}
	}
}
