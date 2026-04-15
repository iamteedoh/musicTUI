package importbackend

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/iamteedoh/musicTUI/internal/config"
)

// SessionFileName is the filename inside the musicTUI config dir that
// holds the backend session credentials. Stays under the same dir as
// the Spotify credentials so users only need to know one location.
const SessionFileName = "import-session.json"

// LoadSession reads a previously-saved session, or returns nil (no
// error) if the file doesn't exist. Any malformed file is treated as
// "no session" — better to start fresh than die on launch.
func LoadSession() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		// Unparseable — behave as if absent so the next call mints a
		// new session and overwrites the corrupt file.
		return nil, nil
	}
	if s.SessionID == "" || s.BackendURL == "" {
		return nil, nil
	}
	return &s, nil
}

// SaveSession writes the session credentials to disk. Creates the
// directory tree if needed. Mode 0600 because the CSRF token is
// effectively a write-credential against the user's library.
func SaveSession(s *Session) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ClearSession removes the persisted session — used by "Disconnect"
// or when the backend reports the session has expired.
func ClearSession() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sessionPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SessionFileName), nil
}
