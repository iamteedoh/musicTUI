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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/tui"
)

//go:embed bridge-bin/*
var bridgeFS embed.FS

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

func main() {
	cfg := config.Load()
	bridgePath := findBridge()

	if bridgePath == "" {
		fmt.Fprintln(os.Stderr, "Warning: player-bridge not found. Audio playback disabled.")
	}

	app := tui.NewApp(cfg, bridgePath)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
