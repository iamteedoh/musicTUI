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

// Version is the config schema version this build writes. Bump it when an
// older build's stored value has to be interpreted differently, and add the
// step to migrate().
//
//	1 — the theme picker and auto-detection (MUS-32).
const Version = 1

type Config struct {
	Version         int           `toml:"config_version"`
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
		Version: Version,
		// "auto" matches the palette to the terminal's own background —
		// dark-only theming made the app illegible on light terminals
		// (MUS-32). An explicit theme name in config.toml still wins.
		Theme:      "auto",
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

// DirEnvVar overrides the config location from the environment, mirroring
// MUSICTUI_ARTWORK. The --config-dir flag takes precedence over it.
const DirEnvVar = "MUSICTUI_CONFIG_DIR"

// dirOverride is set by SetDir. Empty means "use the OS default".
var dirOverride string

// SetDir redirects every config path — config.toml, credentials.json and the
// import token store — at dir. Pass "" to restore the OS default.
//
// This exists so a throwaway profile can be pointed at a temp directory: the
// first-run onboarding wizard only opens when no client_id is configured, so
// exercising it otherwise means moving the real config out of the way by hand.
// Call before Load().
func SetDir(dir string) { dirOverride = dir }

func ConfigDir() (string, error) {
	if dirOverride != "" {
		return dirOverride, nil
	}
	if env := os.Getenv(DirEnvVar); env != "" {
		return env, nil
	}
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
	// Read the version the FILE declares, not the default's — an absent
	// config_version means the file predates versioning and needs migrating.
	cfg.Version = 0
	_ = toml.Unmarshal(data, &cfg)
	cfg = migrate(cfg)

	// A config written before the theme key existed (or with it blanked)
	// gets auto-detection, same as a fresh install.
	if cfg.Theme == "" {
		cfg.Theme = "auto"
	}
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

// migrate brings a config written by an older build up to Version.
func migrate(cfg Config) Config {
	// v0 → v1: before the theme picker (MUS-32) nothing in the app could
	// change the theme, so a stored "nord" is the old *default* rather than a
	// decision — and Save() wrote it into every config.toml that has ever
	// existed. Honoring it as an explicit choice would pin every current user
	// to a dark palette and mean auto-detection never runs for exactly the
	// people this ticket is for, so treat it as unset.
	//
	// This can only ever change the look for someone whose terminal reports a
	// light or mid-tone background: on a dark terminal "auto" resolves back to
	// Nord. A theme the user hand-edited to anything other than the old
	// default is a real choice and is left alone, as is any "nord" written by
	// a versioned build — by then the picker existed, so it was chosen.
	if cfg.Version < 1 && cfg.Theme == "nord" {
		cfg.Theme = "auto"
	}
	cfg.Version = Version
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
