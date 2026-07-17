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
	"github.com/iamteedoh/musicTUI/internal/lyrics"
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

	// The encode happens off the event loop, so drive it the way the app does:
	// render once to record the geometry, encode, install, render again.
	_ = app.artwork.View(theme.Nord(), 30, 18)
	work, ok := app.artwork.PendingSixel()
	if !ok {
		t.Fatal("artwork did not request an encode")
	}
	payload, err := components.EncodeSixel(work)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	if !app.artwork.SetSixelPayload(work.URL, work.Cols, work.Rows, payload) {
		t.Fatal("payload rejected")
	}
	_ = app.artwork.View(theme.Nord(), 30, 18)

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
		app.cache.artRows = "seeded"
		app.cache.art = panelMemo{key: "seeded", val: "seeded"}

		m, cmd := app.Update(tea.WindowSizeMsg{Width: 160, Height: 48})
		app = m.(App)

		if app.width != 160 || app.height != 48 {
			t.Fatalf("%s: resize was not recorded", c.name)
		}
		if cmd == nil {
			if c.wantClear {
				t.Errorf("%s: resize returned no command", c.name)
			}
			// Character art repaints itself; only the row cache must be dropped.
			if app.cache.artRows != "" {
				t.Errorf("%s: cached artwork rows survived a resize", c.name)
			}
			continue
		}

		// Sixel sequences the erase before the repaint. tea.Sequence yields an
		// internal sequenceMsg carrying both, in order.
		gotSequence := fmt.Sprintf("%T", cmd()) == "tea.sequenceMsg"
		if gotSequence != c.wantClear {
			t.Errorf("%s: erase-then-repaint on resize = %v, want %v", c.name, gotSequence, c.wantClear)
		}

		// The cover must NOT be un-cached before the erase runs, or it gets
		// painted and immediately wiped. sixelRepaintMsg does that afterwards.
		if c.wantClear {
			if app.cache.artRows == "" && app.cache.art.key == "" {
				t.Errorf("%s: caches were dropped before the screen was erased", c.name)
			}
			m2, _ := app.Update(sixelRepaintMsg{})
			after := m2.(App)
			if after.cache.artRows != "" || after.cache.art.key != "" {
				t.Errorf("%s: sixelRepaintMsg did not force a repaint", c.name)
			}
		}
	}
}

// Encoding a cover costs tens of milliseconds. Doing it in View froze the event
// loop for a frame, and a resize drag — one geometry per pixel of travel —
// jammed it solid: the artwork vanished, "Loading..." stuck, the terminal was
// flooded with 62KB payloads. It must happen in a command instead (MUS-29).
func TestSixelEncodeHappensOffTheEventLoop(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close()
	app.width, app.height = 160, 48
	fakeSixelArtwork(&app)

	// A frame records the geometry the panel needs...
	_ = app.View()
	if _, ok := app.artwork.PendingSixel(); !ok {
		t.Fatal("View did not request an encode")
	}
	// ...and View must NOT have produced a payload itself.
	if seq, _, _ := app.artwork.SixelDraw(); seq != "" {
		t.Fatal("View encoded the cover on the event loop")
	}

	// The tick turns that into a command.
	m, cmd := app.Update(TickMsg{})
	app = m.(App)
	if !app.sixelEncoding {
		t.Fatal("tick did not start an encode")
	}
	if cmd == nil {
		t.Fatal("tick returned no command")
	}

	// Only one encode may be in flight; a second tick must not launch another.
	m, _ = app.Update(TickMsg{})
	app = m.(App)
	if !app.sixelEncoding {
		t.Fatal("encoding flag was cleared without a result")
	}

	// Deliver the payload the way the command would.
	work, _ := app.artwork.PendingSixel()
	payload, err := components.EncodeSixel(work)
	if err != nil {
		t.Fatalf("EncodeSixel: %v", err)
	}
	m, _ = app.Update(SixelEncodedMsg{URL: work.URL, Cols: work.Cols, Rows: work.Rows, Payload: payload})
	app = m.(App)

	if app.sixelEncoding {
		t.Fatal("encoding flag survived the result")
	}
	if app.cache.artRows != "" {
		t.Fatal("a newly encoded cover must force a repaint")
	}
	_ = app.View()
	if seq, _, _ := app.artwork.SixelDraw(); seq == "" {
		t.Fatal("cover was not published after the encode landed")
	}
}

// A resize while a cover is encoding supersedes it. The stale payload must be
// dropped, not painted at the old geometry.
func TestStaleSixelPayloadIsDropped(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close()
	app.width, app.height = 160, 48
	fakeSixelArtwork(&app)
	_ = app.View()

	work, ok := app.artwork.PendingSixel()
	if !ok {
		t.Fatal("no encode requested")
	}

	// The window resizes while the encode is in flight.
	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(App)
	_ = app.View()

	// The in-flight result now describes a geometry nobody wants.
	m, _ = app.Update(SixelEncodedMsg{URL: work.URL, Cols: work.Cols + 7, Rows: work.Rows + 3, Payload: "STALE"})
	app = m.(App)

	if seq, _, _ := app.artwork.SixelDraw(); strings.Contains(seq, "STALE") {
		t.Fatal("a payload for a superseded geometry was published")
	}
	if _, ok := app.artwork.PendingSixel(); !ok {
		t.Fatal("after dropping a stale payload the panel must ask again")
	}
}

// MUS-32: in Settings, Enter/→/l steps the selected setting forward and ←
// steps back. For the Theme row that cycles Auto → dark → medium → light,
// applies the palette live (no restart) and persists the choice.
func TestSettingsThemeCyclingAppliesAndPersists(t *testing.T) {
	config.SetDir(t.TempDir())
	t.Cleanup(func() { config.SetDir("") })
	t.Setenv("COLORFGBG", "") // make auto-detection deterministic under test

	app := NewApp(config.Load(), "", "test")
	app.onboard.Close() // empty config auto-opens the wizard, which captures keys
	app.view = model.ViewSettings
	app.focus = model.FocusContent

	if app.config.Theme != "auto" || app.theme.Name != "Nord" {
		t.Fatalf("fresh app: theme=%q resolved=%q, want auto resolving to Nord",
			app.config.Theme, app.theme.Name)
	}

	// → on the Theme row (row 0): Auto steps to the first built-in.
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRight})
	app = m.(App)
	if app.config.Theme != theme.AllThemes[0] {
		t.Fatalf("after →: config theme %q, want %q", app.config.Theme, theme.AllThemes[0])
	}

	// ← back to Auto, ← again wraps to the last option.
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = m.(App)
	if app.config.Theme != theme.Auto {
		t.Fatalf("after ←: config theme %q, want auto", app.config.Theme)
	}
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = m.(App)
	last := theme.AllThemes[len(theme.AllThemes)-1]
	if app.config.Theme != last {
		t.Fatalf("← from Auto should wrap to %q, got %q", last, app.config.Theme)
	}
	if want := theme.FromName(last).Name; app.theme.Name != want {
		t.Fatalf("theme not applied live: active %q, want %q", app.theme.Name, want)
	}
	if got := config.Load().Theme; got != last {
		t.Fatalf("persisted theme %q, want %q — the choice must survive a restart", got, last)
	}

	// The boolean row still flips with the same keys.
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyDown})
	app = m.(App)
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRight})
	app = m.(App)
	if !app.config.CheckDuplicates {
		t.Fatal("→ on the check-duplicates row must toggle it on")
	}
}

// The Settings panel owns l/←/→ while focused (they change values there);
// everywhere else the global bindings keep them: l toggles lyrics, arrows
// switch panels. Both directions of that contract are load-bearing — the
// global handlers run first and used to swallow the keys before Settings
// ever saw them.
// A failed lyrics fetch must reach the panel as short plain-language text
// with retry/dismiss affordances — not a raw Go error in the status line,
// and not a "Loading lyrics..." that never resolves (MUS-33).
func TestLyricsErrorIsPanelOnlyAndScopedToTrack(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close()
	track := model.Track{ID: "t1", Name: "Song"}
	app.playback.Track = &track
	app.lyrics.Loading = true
	app.lyrics.TrackID = "t1"

	// A stale failure from the previous track is dropped outright.
	m, _ := app.Update(LyricsErrorMsg{TrackID: "t0", Message: "Couldn't reach the lyrics service"})
	app = m.(App)
	if app.lyrics.Error != "" {
		t.Fatal("a stale track's lyrics error clobbered the current track's panel")
	}

	m, _ = app.Update(LyricsErrorMsg{TrackID: "t1", Message: "Couldn't reach the lyrics service"})
	app = m.(App)
	if app.lyrics.Error != "Couldn't reach the lyrics service" {
		t.Fatalf("lyrics.Error = %q, want the friendly message", app.lyrics.Error)
	}
	if app.lyrics.Loading {
		t.Fatal("panel stuck on Loading after the fetch failed")
	}
	if app.status != "" {
		t.Fatalf("a lyrics failure leaked into the status line: %q", app.status)
	}

	// Stale successful results are dropped the same way.
	app.lyrics.TrackID = "t2"
	m, _ = app.Update(LyricsLoadedMsg{TrackID: "t1", Result: &lyrics.Result{Plain: "old words"}})
	app = m.(App)
	if app.lyrics.PlainText == "old words" {
		t.Fatal("a stale track's lyrics result clobbered the current track's panel")
	}
}

// The error banner owns exactly ctrl+r (retry) and esc (dismiss) while
// visible. The playback letters must keep working — the global key switch
// has swallowed view keys twice before (MUS-12, MUS-32) — and a banner that
// isn't rendered must not eat esc.
func TestLyricsErrorBannerKeys(t *testing.T) {
	app := NewApp(config.Config{}, "", "test")
	app.onboard.Close()
	track := model.Track{ID: "t1", Name: "Song"}
	app.playback.Track = &track
	app.lyrics.TrackID = "t1"
	app.lyrics.Error = "Couldn't reach the lyrics service"
	app.view = model.ViewHome // inline lyrics panel is visible (showLyrics defaults on)

	// r while the banner is up still cycles repeat — not stolen for retry.
	repeatBefore := app.queue.Repeat
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app = m.(App)
	if app.queue.Repeat == repeatBefore {
		t.Fatal("r with the lyrics banner visible no longer cycles repeat mode")
	}

	// ctrl+r retries: banner cleared, loading again, fetch command issued.
	m, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	app = m.(App)
	if app.lyrics.Error != "" || !app.lyrics.Loading {
		t.Fatalf("ctrl+r: Error = %q, Loading = %v — want cleared and loading", app.lyrics.Error, app.lyrics.Loading)
	}
	if cmd == nil {
		t.Fatal("ctrl+r did not return a refetch command")
	}

	// Fail again, esc dismisses to the quiet empty state.
	m, _ = app.Update(LyricsErrorMsg{TrackID: "t1", Message: "Couldn't reach the lyrics service"})
	app = m.(App)
	viewBefore := app.view
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(App)
	if app.lyrics.Error != "" {
		t.Fatal("esc did not dismiss the lyrics error banner")
	}
	if app.view != viewBefore {
		t.Fatal("dismissing the banner also navigated the view")
	}

	// Hidden banner (inline lyrics toggled off) must not eat keys: esc falls
	// through to whatever the view does, and the error text stays put.
	app.lyrics.Error = "Couldn't reach the lyrics service"
	app.showLyrics = false
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(App)
	if app.lyrics.Error == "" {
		t.Fatal("esc dismissed a banner that was not on screen")
	}

	// In the full Lyrics view the banner renders regardless of the inline
	// toggle: first esc dismisses, second esc does the view's normal back.
	app.view = model.ViewLyrics
	app.focus = model.FocusContent
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(App)
	if app.lyrics.Error != "" {
		t.Fatal("esc in the Lyrics view did not dismiss the banner first")
	}
	if app.focus != model.FocusContent {
		t.Fatal("the dismissing esc also moved focus")
	}
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = m.(App)
	if app.focus != model.FocusSidebar {
		t.Fatal("esc after dismissal lost its normal back-to-sidebar behavior")
	}
}

func TestSettingsKeysDoNotLeakToGlobalBindings(t *testing.T) {
	config.SetDir(t.TempDir())
	t.Cleanup(func() { config.SetDir("") })
	t.Setenv("COLORFGBG", "")

	app := NewApp(config.Load(), "", "test")
	app.onboard.Close()
	app.view = model.ViewSettings
	app.focus = model.FocusContent

	// l inside Settings cycles the theme and must not touch lyrics.
	lyricsBefore := app.showLyrics
	m, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app = m.(App)
	if app.showLyrics != lyricsBefore {
		t.Fatal("l inside Settings toggled lyrics — the global binding swallowed it")
	}
	if app.config.Theme != theme.AllThemes[0] {
		t.Fatalf("l inside Settings should cycle the theme, config = %q", app.config.Theme)
	}

	// Arrows inside Settings must not switch panels.
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = m.(App)
	if app.focus != model.FocusContent {
		t.Fatalf("← inside Settings moved focus to %v, must stay on the panel", app.focus)
	}

	// Outside Settings the global bindings still win.
	app.view = model.ViewHome
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	app = m.(App)
	if app.showLyrics == lyricsBefore {
		t.Fatal("l outside Settings must still toggle lyrics")
	}
	m, _ = app.Update(tea.KeyMsg{Type: tea.KeyLeft})
	app = m.(App)
	if app.focus != model.FocusSidebar {
		t.Fatalf("← outside Settings must switch panels, focus = %v", app.focus)
	}
}
