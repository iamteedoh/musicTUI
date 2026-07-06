// Package store persists OAuth tokens on disk with symmetric
// encryption. The key lives at <config-dir>/key with 0600 perms;
// the token files at <config-dir>/<service>.json with 0600.
//
// This isn't a secure vault — a local attacker with the user's UID
// can read both the key and the encrypted tokens — but it does
// prevent casual file-system browsing from exposing plaintext
// tokens, and it matches the approach the Python backend used at
// rest (Fernet-encrypted with a Vault-managed key).
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iamteedoh/musicTUI/internal/importcore/oauth"
)

const (
	keyFilename = "key"
	tokExt      = ".json"
)

// FileStore persists tokens under a config directory. Implements
// oauth.Store.
type FileStore struct {
	Dir string // e.g. ~/.config/musicTUI/import
}

// NewFileStore ensures the directory exists (mode 0700) and returns
// a FileStore. Safe to call repeatedly.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	fs := &FileStore{Dir: dir}
	if _, err := fs.loadOrCreateKey(); err != nil {
		return nil, err
	}
	return fs, nil
}

// Load returns the stored token for `service`, or (nil, nil) if
// absent. Errors propagate if the file exists but can't be decrypted.
func (s *FileStore) Load(service string) (*oauth.ServiceToken, error) {
	path := s.pathFor(service)
	enc, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	key, err := s.loadOrCreateKey()
	if err != nil {
		return nil, err
	}
	plain, err := decrypt(key, enc)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", service, err)
	}
	var tok oauth.ServiceToken
	if err := json.Unmarshal(plain, &tok); err != nil {
		return nil, fmt.Errorf("parse %s: %w", service, err)
	}
	return &tok, nil
}

// Save encrypts and writes the token with 0600 perms.
func (s *FileStore) Save(service string, tok *oauth.ServiceToken) error {
	plain, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	key, err := s.loadOrCreateKey()
	if err != nil {
		return err
	}
	ct, err := encrypt(key, plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.pathFor(service), ct, 0o600)
}

// Delete removes a stored token. Idempotent.
func (s *FileStore) Delete(service string) error {
	err := os.Remove(s.pathFor(service))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ─────────────────── internals ───────────────────

func (s *FileStore) pathFor(service string) string {
	return filepath.Join(s.Dir, service+tokExt)
}

func (s *FileStore) loadOrCreateKey() ([]byte, error) {
	p := filepath.Join(s.Dir, keyFilename)
	data, err := os.ReadFile(p)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(string(data))
		if err == nil && len(key) == 32 {
			return key, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	// Generate + persist a fresh 32-byte key.
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decrypt(key, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, payload := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	return gcm.Open(nil, nonce, payload, nil)
}
