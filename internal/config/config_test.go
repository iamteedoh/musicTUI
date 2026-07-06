package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func isolateUserConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("APPDATA", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	return dir
}

func TestDefaultConfig(t *testing.T) {
	cfg := Default()

	if cfg.Theme != "nord" {
		t.Fatalf("Theme = %q, want nord", cfg.Theme)
	}
	if cfg.TickRateMs != 33 {
		t.Fatalf("TickRateMs = %d, want 33", cfg.TickRateMs)
	}
	if cfg.FrameRate != 60 {
		t.Fatalf("FrameRate = %d, want 60", cfg.FrameRate)
	}
	if cfg.Volume != 75 {
		t.Fatalf("Volume = %d, want 75", cfg.Volume)
	}
	if cfg.CheckDuplicates {
		t.Fatal("CheckDuplicates defaults to true, want false")
	}
}

func TestLoadReturnsDefaultsWhenConfigMissing(t *testing.T) {
	isolateUserConfig(t)

	cfg := Load()
	if cfg != Default() {
		t.Fatalf("Load() = %#v, want default %#v", cfg, Default())
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	isolateUserConfig(t)

	want := Default()
	want.Theme = "dracula"
	want.Volume = 42
	want.CheckDuplicates = true
	want.Spotify.ClientID = "test-client-id"
	want.Import.GoogleClientID = "google-client-id"
	want.Import.GoogleClientSecret = "google-client-secret"
	want.Import.SpotifyClientID = "spotify-import-client-id"
	want.Import.SpotifyClientSecret = "spotify-import-client-secret"

	if err := Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got := Load()
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadNormalizesInvalidNumericValues(t *testing.T) {
	isolateUserConfig(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data := []byte("tick_rate_ms = -1\nframe_rate = 0\nvolume = 900\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got := Load()
	if got.TickRateMs != 33 {
		t.Fatalf("TickRateMs = %d, want 33", got.TickRateMs)
	}
	if got.FrameRate != 60 {
		t.Fatalf("FrameRate = %d, want 60", got.FrameRate)
	}
	if got.Volume != 75 {
		t.Fatalf("Volume = %d, want 75", got.Volume)
	}
}

func TestConfigDirUsesOSUserConfigDir(t *testing.T) {
	root := isolateUserConfig(t)

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}

	var want string
	switch runtime.GOOS {
	case "darwin":
		want = filepath.Join(root, "Library", "Application Support", "musicTUI")
	case "windows":
		want = filepath.Join(root, "AppData", "Roaming", "musicTUI")
	default:
		want = filepath.Join(root, ".config", "musicTUI")
	}
	if got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}
