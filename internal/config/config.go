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
// SpotifyClientID is optional: if empty, we reuse SpotifyConfig.ClientID
// (the playback app). Setting it means the user created a dedicated
// Spotify dev app for imports — recommended for heavy use because
// Spotify rate-limits per app, and a burst of imports can otherwise
// throttle playback for hours. The wizard walks users through the
// trade-off.
type ImportConfig struct {
	GoogleClientID      string `toml:"google_client_id"`
	GoogleClientSecret  string `toml:"google_client_secret"`
	SpotifyClientID     string `toml:"spotify_client_id,omitempty"`
	SpotifyClientSecret string `toml:"spotify_client_secret"`
}

// SpotifyImportClientID returns the effective client_id to use for
// the import flow: the dedicated one from ImportConfig if set,
// otherwise the playback app's.
func (c Config) SpotifyImportClientID() string {
	if c.Import.SpotifyClientID != "" {
		return c.Import.SpotifyClientID
	}
	return c.Spotify.ClientID
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
