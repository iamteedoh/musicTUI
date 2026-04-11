package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/iamteedoh/musictui-go/internal/config"
	"github.com/iamteedoh/musictui-go/internal/tui"
)

func findBridge() string {
	// Check next to this binary first
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, "player-bridge")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Check the Rust project's release build
	home, _ := os.UserHomeDir()
	candidate := filepath.Join(home, "git", "musicTUI", "target", "release", "player-bridge")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}

	// Check PATH
	if path, err := exec.LookPath("player-bridge"); err == nil {
		return path
	}

	return ""
}

func main() {
	cfg := config.Load()
	bridgePath := findBridge()

	if bridgePath == "" {
		fmt.Fprintln(os.Stderr, "Warning: player-bridge not found. Audio playback disabled.")
		fmt.Fprintln(os.Stderr, "Build it: cd ~/git/musicTUI && cargo build --bin player-bridge --release")
	}

	app := tui.NewApp(cfg, bridgePath)

	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
