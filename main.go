package main

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"image"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/termcap"
	"github.com/iamteedoh/musicTUI/internal/theme"
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
	fmt.Println("  musicTUI --artwork-probe [FILE|URL]")
	fmt.Println("                             Send an image through the kitty-graphics")
	fmt.Println("                             artwork pipeline with terminal responses")
	fmt.Println("                             enabled, and report the terminal's verdict")
	fmt.Println("  musicTUI --config-dir DIR  Use DIR for config, credentials and")
	fmt.Println("                             import tokens instead of the default")
	fmt.Println("\nPoint --config-dir at an empty directory to get a clean first run")
	fmt.Println("(the setup wizard) without touching your real configuration. The")
	fmt.Println("MUSICTUI_CONFIG_DIR environment variable does the same thing.")
	fmt.Println("\nArtwork rendering can be forced with the MUSICTUI_ARTWORK")
	fmt.Println("environment variable: kitty | sixel | iterm2 | blocks | braille.")
	fmt.Println("Run `musicTUI --caps` inside a terminal to see what it supports.")
	fmt.Println("\nWhen a specific cover won't render, set MUSICTUI_ARTWORK_DEBUG=1 to")
	fmt.Println("log every artwork decision (renderer choice, transmit sizes, image")
	fmt.Println("ids, fallbacks) to <cache>/musicTUI/artwork-debug.log — or set it to")
	fmt.Println("a file path of your choice. The log records each cover's URL, which")
	fmt.Println("`musicTUI --artwork-probe <URL>` can then replay in the same terminal")
	fmt.Println("to capture the rejection reason the app itself must suppress.")
}

// printCaps reports what the terminal said about itself and which artwork
// renderer that selects. Run it inside the terminal you're diagnosing.
func printCaps() {
	caps := termcap.Detect()
	detected := components.DetectArtworkStyle(caps.Kitty, caps.Sixel)
	style := "blocks (character art)"
	switch detected {
	case components.StyleKitty:
		style = "kitty graphics (real pixels)"
	case components.StyleSixel:
		style = "sixel graphics (real pixels)"
	case components.StyleITerm2:
		style = "iTerm2 inline images (real pixels)"
	case components.StyleBraille:
		style = "braille (character art)"
	}

	// What theme = "auto" would pick here — the probe's background answer,
	// falling back to COLORFGBG, falling back to dark (MUS-32).
	bg := caps.Bg
	if bg == "" {
		bg = "(no reply)"
	}
	auto := theme.Resolve(theme.Auto, caps.Bg)

	fmt.Printf("musicTUI %s — terminal capabilities\n\n", Version)
	fmt.Printf("  TERM           %s\n", os.Getenv("TERM"))
	fmt.Printf("  TERM_PROGRAM   %s\n", os.Getenv("TERM_PROGRAM"))
	fmt.Printf("  kitty graphics %t\n", caps.Kitty)
	fmt.Printf("  sixel graphics %t\n", caps.Sixel)
	fmt.Printf("  cell size      %dx%d px\n", caps.CellW, caps.CellH)
	fmt.Printf("  artwork        %s\n", style)
	fmt.Printf("  background     %s\n", bg)
	fmt.Printf("  auto theme     %s (%s)\n", auto.Name, auto.Tier)
	if p := components.ArtworkDebugPath(); p != "" {
		fmt.Printf("  artwork debug  %s\n", p)
	}
	fmt.Printf("\n  raw reply      %s\n", caps.RawEscaped())

	if caps.CellW == 0 || caps.CellH == 0 {
		fmt.Println("\n  No usable cell size: the terminal answered neither CSI 16 t nor an")
		fmt.Println("  exact CSI 14 t / CSI 18 t pair. Sixel is disabled — an image scaled")
		fmt.Println("  against a guessed cell size would spill outside its panel.")
		if detected == components.StyleITerm2 {
			fmt.Println("  The iTerm2 tier is unaffected: its images are sized in cells.")
		}
	}
}

// loadProbeImage resolves the --artwork-probe source: an http(s) URL (a cover
// URL from the artwork debug log), a local image file, or — with no source —
// a generated cover-sized test image that separates "this terminal's kitty
// pipeline is broken" from "this particular cover is rejected".
func loadProbeImage(src string) (image.Image, string, error) {
	if src == "" {
		return components.ProbeTestImage(), "(generated 640×640 test image)", nil
	}
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		res := components.FetchArtwork(src)
		if res.Err != "" {
			return nil, "", fmt.Errorf("fetching %s: %s", src, res.Err)
		}
		return res.Img, src, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, "", fmt.Errorf("decoding %s: %w", src, err)
	}
	return img, src, nil
}

// runArtworkProbe replays the kitty artwork pipeline for one image with
// terminal responses enabled and prints what the terminal said. This exists
// because the app must send its graphics escapes with responses suppressed
// (a reply would be parsed as keystrokes), so a terminal that rejects a
// specific cover does it silently — the panel stays empty and the reason is
// discarded. Run this inside the misbehaving terminal.
func runArtworkProbe(src string) {
	img, label, err := loadProbeImage(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	session, err := termcap.OpenProbeSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "The probe talks to the terminal directly; run it interactively, outside tmux.")
		os.Exit(1)
	}
	report := components.ProbeKittyArtwork(label, img, session)
	session.Close()

	printProbeReport(label, report)
}

func printProbeReport(label string, r components.KittyProbeReport) {
	fmt.Printf("musicTUI %s — kitty graphics probe\n\n", Version)
	fmt.Printf("  source        %s\n", label)
	fmt.Printf("  decoded       %dx%d px\n", r.SrcW, r.SrcH)

	if r.EncodeErr != nil {
		fmt.Printf("\n  PNG ENCODE FAILED: %v\n\n", r.EncodeErr)
		fmt.Println("  Nothing was sent to the terminal. The app falls back to block art")
		fmt.Println("  for this cover (the panel shows chunky colored blocks, not pixels).")
		return
	}
	fmt.Printf("  image id      %d\n", r.ID)
	fmt.Printf("  png payload   %d bytes → %d base64 chars in %d chunks\n", r.PNGBytes, r.B64Chars, r.Chunks)
	fmt.Printf("  placement     %dx%d cells (virtual)\n", r.Cols, r.Rows)
	fmt.Printf("  color profile %s\n", components.ColorProfileName())
	if r.WriteErr != nil {
		fmt.Printf("\n  WRITE FAILED: %v\n", r.WriteErr)
		return
	}

	fmt.Printf("\n  transmit      %s\n", replyOrNone(r.TransmitReply))
	fmt.Printf("  placement     %s\n", replyOrNone(r.PlacementReply))
	fmt.Println()

	switch {
	case r.TransmitOK() && r.PlacementOK():
		fmt.Println("  The terminal accepted this cover — it stored the image and bound a")
		fmt.Println("  virtual placement to it. Below is the same placeholder text the app")
		fmt.Println("  prints; the terminal draws the image over those cells itself:")
		fmt.Println()
		fmt.Println(r.PlaceholderGrid())
		fmt.Println()
		fmt.Println("  The cover above: the whole kitty pipeline works for this image, and")
		fmt.Println("  a blank panel in the app is NOT this terminal refusing the picture.")
		fmt.Println("  Literal glyphs (boxes/tofu) above, despite both OKs: the fault is in")
		fmt.Println("  the placeholder text layer, not the image transfer — most likely the")
		fmt.Println("  image id carried in the foreground color, which anything below a")
		fmt.Println("  TrueColor profile quantizes into a different (nonexistent) id.")
		if p := components.ColorProfileName(); p != "TrueColor" {
			fmt.Printf("  NOTE: the color profile here is %s — that alone corrupts the id.\n", p)
		}
		fmt.Printf("\n  Image %d stays in this terminal's memory until you close the window;\n", r.ID)
		fmt.Println("  deleting it here would erase the very grid printed above.")
	case r.TransmitReply == "" && r.PlacementReply == "":
		fmt.Println("  The terminal never answered. It most likely does not implement the")
		fmt.Println("  kitty graphics protocol — run `musicTUI --caps` to see what it")
		fmt.Println("  advertises and which artwork renderer musicTUI actually picks here.")
	default:
		fmt.Println("  The terminal REJECTED this image — the reply above is the reason")
		fmt.Println("  this cover never renders in the app. The app cannot see or show")
		fmt.Println("  this error: it must suppress graphics responses (q=2), because a")
		fmt.Println("  reply would be parsed as keystrokes.")
	}

	fmt.Printf("\n  raw reply     %s\n", termcap.EscapeControls(r.Raw))
}

func replyOrNone(reply string) string {
	if reply == "" {
		return "(no reply)"
	}
	return reply
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

		// Replay the artwork pipeline for one image with responses enabled.
		// The optional source is a cover URL (from the artwork debug log) or
		// a local image file; without one, a generated test image is sent.
		case arg == "--artwork-probe":
			src := ""
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				src = args[i]
			}
			runArtworkProbe(src)
			return false

		case strings.HasPrefix(arg, "--artwork-probe="):
			runArtworkProbe(strings.TrimPrefix(arg, "--artwork-probe="))
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
