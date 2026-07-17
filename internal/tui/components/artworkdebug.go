package components

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Artwork debug log (MUS-34).
//
// The kitty tier must send its graphics escapes with responses suppressed
// (q=2 — a terminal reply would land in Bubble Tea's input stream as
// keystrokes), so when a terminal rejects a cover it does so silently and the
// panel just stays empty. This log is the app-side half of the diagnosis: an
// append-only trace of every decision the artwork pipeline makes — which
// renderer was chosen and why, what was transmitted (size, chunk count, image
// id, grid), and whether a PNG encode failed and fell back to block art. The
// terminal-side half is `musicTUI --artwork-probe`, which replays the same
// escapes with responses enabled.
//
// Enabled by MUSICTUI_ARTWORK_DEBUG: "1"/"true"/"yes"/"on" logs to
// <user cache dir>/musicTUI/artwork-debug.log; any other non-empty value is
// itself the log file path. Writing to a file (never the terminal) keeps the
// log out of the TUI and of the escape streams it is describing.

var (
	artworkDebugOnce sync.Once
	artworkDebugMu   sync.Mutex
	artworkDebugFile *os.File
	artworkDebugDest string
)

func artworkDebugInit() {
	artworkDebugOnce.Do(func() {
		dest := os.Getenv("MUSICTUI_ARTWORK_DEBUG")
		switch strings.ToLower(dest) {
		case "", "0", "false", "no", "off":
			return
		case "1", "true", "yes", "on":
			cacheDir, err := os.UserCacheDir()
			if err != nil {
				return
			}
			dest = filepath.Join(cacheDir, "musicTUI", "artwork-debug.log")
		}
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		f, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		artworkDebugFile = f
		artworkDebugDest = dest
	})
}

// ArtworkDebugPath reports where the artwork debug log is being written, or
// "" when MUSICTUI_ARTWORK_DEBUG is unset (or its destination couldn't be
// opened). `--caps` prints it so users know where to look.
func ArtworkDebugPath() string {
	artworkDebugInit()
	return artworkDebugDest
}

// artworkDebugf appends one timestamped line to the artwork debug log.
// A no-op unless MUSICTUI_ARTWORK_DEBUG enabled the log, so call sites can
// stay unconditional.
func artworkDebugf(format string, args ...any) {
	artworkDebugInit()
	if artworkDebugFile == nil {
		return
	}
	artworkDebugMu.Lock()
	defer artworkDebugMu.Unlock()
	fmt.Fprintf(artworkDebugFile, time.Now().Format("2006-01-02 15:04:05.000")+" "+format+"\n", args...)
}

// artworkDebugReset closes the log and re-reads MUSICTUI_ARTWORK_DEBUG on the
// next call — tests only.
func artworkDebugReset() {
	artworkDebugMu.Lock()
	defer artworkDebugMu.Unlock()
	if artworkDebugFile != nil {
		_ = artworkDebugFile.Close()
	}
	artworkDebugFile = nil
	artworkDebugDest = ""
	artworkDebugOnce = sync.Once{}
}

// ColorProfileName names the color profile lipgloss will render styles with.
// It matters to the kitty tier specifically: the image id rides in the
// placeholder cells' foreground color, so anything below TrueColor quantizes
// the id and the placeholders reference an image that doesn't exist.
func ColorProfileName() string {
	switch lipgloss.ColorProfile() {
	case termenv.TrueColor:
		return "TrueColor"
	case termenv.ANSI256:
		return "ANSI256"
	case termenv.ANSI:
		return "ANSI"
	case termenv.Ascii:
		return "Ascii (no color)"
	}
	return "unknown"
}
