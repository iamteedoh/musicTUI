package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type SpotifyConfig struct {
	ClientID string `toml:"client_id"`
}

// AppleMusicConfig holds the user-supplied values needed for the
// Apple Music import path. All fields optional — when the developer
// token or auth page URL are empty, the Import view treats Apple
// Music as "not configured" and shows setup instructions instead of
// attempting an auth flow.
//
// Why this is in config rather than obtained automatically:
//   - DeveloperToken is a JWT signed by an Apple Developer private
//     key (.p8). Generating it requires paid Apple Developer
//     membership; musicTUI can't do it.
//   - AuthPageURL is the HTTPS URL where the static MusicKit JS page
//     (page/apple-auth/index.html) is hosted. User-specific.
type AppleMusicConfig struct {
	DeveloperToken string `toml:"developer_token"`
	AuthPageURL    string `toml:"auth_page_url"`
	CallbackPort   int    `toml:"callback_port"` // 0 = auto-pick
}

type Config struct {
	Theme               string           `toml:"theme"`
	TickRateMs          int              `toml:"tick_rate_ms"`
	FrameRate           int              `toml:"frame_rate"`
	Volume              int              `toml:"volume"`
	CheckDuplicates     bool             `toml:"check_duplicates"`
	Spotify             SpotifyConfig    `toml:"spotify"`
	AppleMusic          AppleMusicConfig `toml:"apple_music"`
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
