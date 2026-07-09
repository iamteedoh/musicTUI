package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/termcap"
	"github.com/iamteedoh/musicTUI/internal/tui"
	"github.com/iamteedoh/musicTUI/internal/tui/components"
)

//go:embed bridge-bin/*
var bridgeFS embed.FS

// Version is injected at build time via -ldflags "-X main.Version=...".
// Falls back to "dev" for local builds.
var Version = "dev"

func findBridge() string {
	// 1. Try extracting the embedded bridge binary
	if path := extractEmbeddedBridge(); path != "" {
		return path
	}

	// 2. Check next to this binary
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, bridgeName())
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// 3. Check PATH
	if path, err := exec.LookPath(bridgeName()); err == nil {
		return path
	}

	return ""
}

func bridgeName() string {
	if runtime.GOOS == "windows" {
		return "player-bridge.exe"
	}
	return "player-bridge"
}

func extractEmbeddedBridge() string {
	name := bridgeName()
	data, err := fs.ReadFile(bridgeFS, "bridge-bin/"+name)
	if err != nil || len(data) == 0 {
		return ""
	}

	// Cache in ~/.cache/musicTUI/bin/
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	binDir := filepath.Join(cacheDir, "musicTUI", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return ""
	}

	dest := filepath.Join(binDir, name)

	// Only write if the file doesn't exist or the hash changed (new version)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	hashFile := dest + ".sha256"
	if existing, err := os.ReadFile(hashFile); err == nil && string(existing) == hash {
		// Same version already extracted
		if _, err := os.Stat(dest); err == nil {
			return dest
		}
	}

	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return ""
	}
	_ = os.WriteFile(hashFile, []byte(hash), 0o644)
	return dest
}

func usage() {
	fmt.Println("musicTUI — a terminal music player for Spotify")
	fmt.Println("\nUsage:")
	fmt.Println("  musicTUI                   Launch the player")
	fmt.Println("  musicTUI --version         Print the version and exit")
	fmt.Println("  musicTUI --caps            Report what this terminal supports and exit")
	fmt.Println("  musicTUI --config-dir DIR  Use DIR for config, credentials and")
	fmt.Println("                             import tokens instead of the default")
	fmt.Println("\nPoint --config-dir at an empty directory to get a clean first run")
	fmt.Println("(the setup wizard) without touching your real configuration. The")
	fmt.Println("MUSICTUI_CONFIG_DIR environment variable does the same thing.")
	fmt.Println("\nArtwork rendering can be forced with the MUSICTUI_ARTWORK")
	fmt.Println("environment variable: kitty | sixel | blocks | braille.")
	fmt.Println("Run `musicTUI --caps` inside a terminal to see what it supports.")
}

// printCaps reports what the terminal said about itself and which artwork
// renderer that selects. Run it inside the terminal you're diagnosing.
func printCaps() {
	caps := termcap.Detect()
	style := "blocks (character art)"
	switch components.DetectArtworkStyle(caps.Kitty, caps.Sixel) {
	case components.StyleKitty:
		style = "kitty graphics (real pixels)"
	case components.StyleSixel:
		style = "sixel graphics (real pixels)"
	case components.StyleBraille:
		style = "braille (character art)"
	}

	fmt.Printf("musicTUI %s — terminal capabilities\n\n", Version)
	fmt.Printf("  TERM           %s\n", os.Getenv("TERM"))
	fmt.Printf("  TERM_PROGRAM   %s\n", os.Getenv("TERM_PROGRAM"))
	fmt.Printf("  kitty graphics %t\n", caps.Kitty)
	fmt.Printf("  sixel graphics %t\n", caps.Sixel)
	fmt.Printf("  cell size      %dx%d px\n", caps.CellW, caps.CellH)
	fmt.Printf("  artwork        %s\n", style)
	fmt.Printf("\n  raw reply      %s\n", caps.RawEscaped())

	if caps.CellW == 0 || caps.CellH == 0 {
		fmt.Println("\n  No usable cell size: the terminal answered neither CSI 16 t nor an")
		fmt.Println("  exact CSI 14 t / CSI 18 t pair. Sixel is disabled — an image scaled")
		fmt.Println("  against a guessed cell size would spill outside its panel.")
	}
}

// parseArgs handles the lightweight CLI flags accepted before the TUI starts.
// It reports whether main should carry on and launch the player.
func parseArgs(args []string) (run bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		// `--version` lets a user confirm which build they're running (MUS-20);
		// the same string is shown in the player's title bar.
		case arg == "--version", arg == "-v", arg == "version":
			fmt.Printf("musicTUI %s\n", Version)
			return false

		case arg == "--help", arg == "-h":
			usage()
			return false

		// Terminals disagree wildly about the graphics and cell-size queries,
		// and artwork problems are almost always a disagreement we can't see.
		// Print exactly what this terminal answered.
		case arg == "--caps":
			printCaps()
			return false

		case arg == "--config-dir":
			if i+1 >= len(args) || args[i+1] == "" {
				fmt.Fprintln(os.Stderr, "Error: --config-dir requires a directory")
				os.Exit(2)
			}
			i++
			config.SetDir(args[i])

		case strings.HasPrefix(arg, "--config-dir="):
			dir := strings.TrimPrefix(arg, "--config-dir=")
			if dir == "" {
				fmt.Fprintln(os.Stderr, "Error: --config-dir requires a directory")
				os.Exit(2)
			}
			config.SetDir(dir)

		default:
			// Never launch on a typo: an unrecognised flag would otherwise
			// silently fall through and write to the real config directory.
			fmt.Fprintf(os.Stderr, "Error: unknown argument %q\n\n", arg)
			usage()
			os.Exit(2)
		}
	}
	return true
}

func main() {
	if !parseArgs(os.Args[1:]) {
		return
	}

	cfg := config.Load()
	bridgePath := findBridge()

	if bridgePath == "" {
		fmt.Fprintln(os.Stderr, "Warning: player-bridge not found. Audio playback disabled.")
	}

	app := tui.NewApp(cfg, bridgePath, Version)

	// One writer for both the renderer and the out-of-band graphics payloads,
	// so a sixel image can never interleave with — or be overwritten by — the
	// frame that reserves its cells.
	out := tui.NewTermWriter(os.Stdout)
	app.SetOutput(out)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithOutput(out))
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
