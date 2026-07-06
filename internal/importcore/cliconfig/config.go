// Package cliconfig handles on-disk configuration for the
// standalone musictui-import CLI: OAuth client credentials and
// defaults. Kept in an internal/ package so external consumers
// (e.g. the musicTUI TUI that imports this module) don't pick up
// the CLI's config shape by accident.
package cliconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the on-disk config shape. JSON instead of TOML so we
// don't pull in a TOML library — config is small and human-
// editable regardless.
type Config struct {
	Google struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"google"`
	Spotify struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"spotify"`
}

// IsReady reports whether both services have creds configured.
func (c Config) IsReady() bool {
	return c.Google.ClientID != "" && c.Google.ClientSecret != "" &&
		c.Spotify.ClientID != "" && c.Spotify.ClientSecret != ""
}

// Missing returns the list of human-readable fields that still need
// to be configured (empty string in either client_id/client_secret).
func (c Config) Missing() []string {
	var out []string
	if c.Google.ClientID == "" {
		out = append(out, "google.client_id")
	}
	if c.Google.ClientSecret == "" {
		out = append(out, "google.client_secret")
	}
	if c.Spotify.ClientID == "" {
		out = append(out, "spotify.client_id")
	}
	if c.Spotify.ClientSecret == "" {
		out = append(out, "spotify.client_secret")
	}
	return out
}

// Dir returns the standalone CLI's config directory
// (~/.config/musictui-import). Respects XDG_CONFIG_HOME.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "musictui-import"), nil
}

// Path returns the absolute path of the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk. Returns a zero Config + nil on
// first-run (file doesn't exist); real errors (permission denied,
// malformed JSON) propagate.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return &cfg, nil
}

// Save writes the config to disk with 0600 perms (contains OAuth
// client secrets — not strictly secret in the desktop-app OAuth
// model, but restricting access to the user's UID is hygiene).
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}
