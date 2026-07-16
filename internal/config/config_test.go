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

	// "auto" adapts the palette to the terminal background (MUS-32) — a
	// fresh install must never pin a dark theme onto a light terminal.
	if cfg.Theme != "auto" {
		t.Fatalf("Theme = %q, want auto", cfg.Theme)
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

// Every config.toml ever written by a pre-MUS-32 build carries theme = "nord"
// — the old default, which Save() persisted whether or not the user cared,
// and which nothing in the app could change. Honoring it as a choice pinned
// every existing user to a dark palette and meant auto-detection only ever
// ran on fresh installs: the exact opposite of what MUS-32 is for.
func TestLoadMigratesPreThemePickerDefaultToAuto(t *testing.T) {
	isolateUserConfig(t)
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
	}

	// Exactly what a pre-MUS-32 install has on disk: no config_version, and
	// the old default theme. Must become auto so detection can run.
	write("theme = \"nord\"\ntick_rate_ms = 33\nvolume = 75\n")
	got := Load()
	if got.Theme != "auto" {
		t.Fatalf("Theme = %q for an unversioned config, want auto — existing users never get the MUS-32 fix otherwise", got.Theme)
	}
	if got.Version != Version {
		t.Fatalf("Version = %d after migration, want %d", got.Version, Version)
	}
	// Unrelated settings must survive the migration untouched.
	if got.Volume != 75 || got.TickRateMs != 33 {
		t.Fatalf("migration disturbed other settings: %#v", got)
	}

	// A theme hand-edited to anything but the old default is a real choice.
	write("theme = \"dracula\"\n")
	if got := Load().Theme; got != "dracula" {
		t.Fatalf("Theme = %q, a hand-picked theme must never be migrated", got)
	}

	// Once a versioned build has written the config, the picker existed —
	// so "nord" there IS a deliberate choice and must stick.
	write("config_version = 1\ntheme = \"nord\"\n")
	if got := Load().Theme; got != "nord" {
		t.Fatalf("Theme = %q for a versioned config, want nord — an explicit pick must not be re-migrated", got)
	}
}

// Choosing Nord in the picker must survive a restart — the migration must not
// reinterpret it as the old default on the next launch.
func TestExplicitNordChoiceSurvivesRestart(t *testing.T) {
	isolateUserConfig(t)

	cfg := Default()
	cfg.Theme = "nord" // as the settings picker would store it
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got := Load().Theme; got != "nord" {
		t.Fatalf("Theme = %q after save/reload, want nord — the picker's choice was migrated away", got)
	}
}

// A config that predates the theme key (or blanked it) must get
// auto-detection, while an explicit choice is always preserved.
func TestLoadNormalizesTheme(t *testing.T) {
	isolateUserConfig(t)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(path, []byte("theme = \"\"\nvolume = 50\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := Load().Theme; got != "auto" {
		t.Fatalf("Theme = %q after loading an empty theme key, want auto", got)
	}

	if err := os.WriteFile(path, []byte("theme = \"gruvbox\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if got := Load().Theme; got != "gruvbox" {
		t.Fatalf("Theme = %q, an explicit theme choice must survive Load", got)
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

// --config-dir / MUSICTUI_CONFIG_DIR let a throwaway profile be pointed at a
// temp directory, so the first-run onboarding wizard can be exercised without
// moving the real config out of the way (MUS-23).
func TestConfigDirOverridePrecedence(t *testing.T) {
	t.Cleanup(func() { SetDir("") })

	osDefault, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() with no override: %v", err)
	}

	envDir := t.TempDir()
	t.Setenv(DirEnvVar, envDir)
	if got, _ := ConfigDir(); got != envDir {
		t.Fatalf("ConfigDir() = %q, want the %s value %q", got, DirEnvVar, envDir)
	}

	flagDir := t.TempDir()
	SetDir(flagDir)
	if got, _ := ConfigDir(); got != flagDir {
		t.Fatalf("ConfigDir() = %q, want the --config-dir value %q (flag must beat env)", got, flagDir)
	}

	// Every path derives from the override, so credentials and import tokens
	// stay isolated too — not just config.toml.
	if got, _ := ConfigPath(); got != filepath.Join(flagDir, "config.toml") {
		t.Fatalf("ConfigPath() = %q, not under the override", got)
	}
	if got, _ := CredentialsPath(); got != filepath.Join(flagDir, "credentials.json") {
		t.Fatalf("CredentialsPath() = %q, not under the override", got)
	}

	SetDir("")
	os.Unsetenv(DirEnvVar)
	if got, _ := ConfigDir(); got != osDefault {
		t.Fatalf("ConfigDir() = %q after clearing overrides, want %q", got, osDefault)
	}
}

// A round-trip through the override directory must actually create it and
// persist the value — this is the path a throwaway test profile exercises.
func TestSaveLoadUsesOverrideDir(t *testing.T) {
	t.Cleanup(func() { SetDir("") })
	dir := filepath.Join(t.TempDir(), "fresh")
	SetDir(dir)

	if got := Load().Spotify.ClientID; got != "" {
		t.Fatalf("a fresh override dir must look like a first run, got client_id %q", got)
	}

	cfg := Default()
	cfg.Spotify.ClientID = "abc123"
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() into a non-existent override dir: %v", err)
	}
	if got := Load().Spotify.ClientID; got != "abc123" {
		t.Fatalf("Load() = %q, want abc123", got)
	}
}
