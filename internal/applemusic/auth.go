package applemusic

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Apple Music authentication needs two tokens:
//  1. Developer Token: a JWT signed with the user's Apple Developer
//     private key (.p8 file). Lives up to 180 days. Generated once
//     and configured in musicTUI's config — see README / future
//     Settings UI for the value.
//  2. Music User Token: issued by MusicKit after the *user* signs in.
//     Cannot be obtained from Go directly — MusicKit JS/iOS/macOS
//     only. We receive it via a local HTTP callback from the hosted
//     MusicKit JS auth page.

// Credentials bundles both tokens once the user has signed in.
// DeveloperToken is static (from config); MusicUserToken is per-session
// and expires after ~6 months.
type Credentials struct {
	DeveloperToken string    `json:"developer_token"`
	MusicUserToken string    `json:"music_user_token"`
	ObtainedAt     time.Time `json:"obtained_at"`
}

// TokenPath returns the disk location where Apple Music credentials
// are persisted. Mirrors the Spotify / YT Music pattern.
func TokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "musicTUI", "applemusic-credentials.json"), nil
}

// SaveCredentials persists the credentials to disk with 0600 perms.
func SaveCredentials(c *Credentials) error {
	p, err := TokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(c)
}

// LoadCredentials reads credentials from disk. Returns nil, nil if
// the file doesn't exist — treat that as "not authed".
func LoadCredentials() (*Credentials, error) {
	p, err := TokenPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var c Credentials
	if err := json.NewDecoder(f).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ClearCredentials removes the cached credentials, used when the user
// signs out or the tokens expire.
func ClearCredentials() error {
	p, err := TokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CallbackResult is what the local callback server returns: the
// Music User Token POSTed by the MusicKit JS page, along with a
// nonce to verify it's the expected session.
type CallbackResult struct {
	MusicUserToken string
	State          string
	Err            error
}

// StartCallbackServer launches a tiny HTTP listener on 127.0.0.1 for
// the MusicKit JS page to POST the Music User Token into. Returns a
// channel that yields exactly one CallbackResult when the page
// responds (or the context is cancelled). The URL to tell the browser
// page is returned as well.
//
// We generate a random `state` each session and require the page to
// echo it back, preventing the local server from being hijacked by
// another process / browser tab during the window it's open.
func StartCallbackServer(ctx context.Context, port int) (callbackURL string, state string, ch <-chan CallbackResult, err error) {
	state, err = randomState()
	if err != nil {
		return "", "", nil, err
	}
	resultCh := make(chan CallbackResult, 1)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return "", "", nil, fmt.Errorf("listen: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/applemusic/callback", func(w http.ResponseWriter, r *http.Request) {
		// Allow the remote auth page (on doralab or wherever) to POST
		// cross-origin. The token is opaque and only useful to this
		// specific user anyway; any origin that can reach us can send
		// it, but the state nonce gates acceptance.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			MusicUserToken string `json:"music_user_token"`
			State          string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if body.State != state {
			http.Error(w, "state mismatch", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))

		select {
		case resultCh <- CallbackResult{MusicUserToken: body.MusicUserToken, State: body.State}:
		default:
			// drained or closed — ignore
		}
	})

	srv := &http.Server{Handler: mux, ReadTimeout: 30 * time.Second}
	go func() { _ = srv.Serve(listener) }()
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	addr := listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf("http://127.0.0.1:%d/applemusic/callback", addr.Port), state, resultCh, nil
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
