package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type SpotifyConfig struct {
	ClientID string `toml:"client_id"`
}

type Config struct {
	Theme               string        `toml:"theme"`
	TickRateMs          int           `toml:"tick_rate_ms"`
	FrameRate           int           `toml:"frame_rate"`
	Volume              int           `toml:"volume"`
	CheckDuplicates     bool          `toml:"check_duplicates"`
	Spotify             SpotifyConfig `toml:"spotify"`
}

func Default() Config {
	return Config{
		Theme:           "nord",
		TickRateMs:      33,
		FrameRate:       60,
		Volume:          75,
		CheckDuplicates: true,
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
