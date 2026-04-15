package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type SpotifyConfig struct {
	ClientID string `toml:"client_id"`
}

// ImportConfig holds user-supplied OAuth client credentials for the
// embedded library-import feature. No hosted service — the binary
// runs OAuth loopback flows against Google Cloud and Spotify using
// these creds directly.
//
// Spotify: reuse the same client_id from SpotifyConfig above. The
// ClientSecret field here is needed because plain Authorization
// Code exchange requires it. Playback uses PKCE and doesn't need
// the secret — but we ask for it here so import can work.
type ImportConfig struct {
	GoogleClientID     string `toml:"google_client_id"`
	GoogleClientSecret string `toml:"google_client_secret"`
	// Spotify import-side client secret. Same app as SpotifyConfig.ClientID.
	SpotifyClientSecret string `toml:"spotify_client_secret"`
}

type Config struct {
	Theme           string        `toml:"theme"`
	TickRateMs      int           `toml:"tick_rate_ms"`
	FrameRate       int           `toml:"frame_rate"`
	Volume          int           `toml:"volume"`
	CheckDuplicates bool          `toml:"check_duplicates"`
	Spotify         SpotifyConfig `toml:"spotify"`
	Import          ImportConfig  `toml:"import"`
}

func Default() Config {
	return Config{
		Theme:      "nord",
		TickRateMs: 33,
		FrameRate:  60,
		Volume:     75,
		// Off by default. The "cleanup" it offers unfollows playlists
		// (Spotify's only "delete" operation), which for playlists the
		// user owns results in them disappearing from /me/playlists
		// without any real way to recover via the public API. Users who
		// want this can opt-in via Settings.
		CheckDuplicates: false,
	}
}

func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "musicTUI"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func CredentialsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func Load() Config {
	cfg := Default()
	path, err := ConfigPath()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	_ = toml.Unmarshal(data, &cfg)
	if cfg.FrameRate <= 0 {
		cfg.FrameRate = 60
	}
	if cfg.TickRateMs <= 0 {
		cfg.TickRateMs = 33
	}
	if cfg.Volume < 0 || cfg.Volume > 100 {
		cfg.Volume = 75
	}
	return cfg
}

func Save(cfg Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
