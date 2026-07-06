// Package oauth implements OAuth 2.0 flows for Google (YouTube Data
// API v3) and Spotify Web API using a loopback redirect — a tiny
// HTTP server on 127.0.0.1:<random-port> that catches the callback
// with the authorization code.
//
// This pattern avoids needing a hosted service. The user's browser
// talks to the provider, the provider redirects to our loopback,
// we exchange the code for tokens locally, browser shows "you can
// close this tab".
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// CallbackResult is what LoopbackListen returns after the provider
// redirects back. Exactly one of Code or Err is populated.
type CallbackResult struct {
	Code  string
	State string
	Err   error
}

// Loopback is a one-shot HTTP server that catches a single OAuth
// callback and shuts itself down. Safe to use even in a binary that
// might run multiple OAuth flows in the same process — each call to
// Listen gets its own port + server.
type Loopback struct {
	Port   int
	URL    string // full redirect URI: http://127.0.0.1:<port>/callback
	result chan CallbackResult
	server *http.Server
}

// Listen starts a local HTTP server on `port` on 127.0.0.1.
// Pass port=0 to let the OS pick a random free port.
//
// Spotify requires the redirect URI registered in your dev app to
// match exactly (including the port), so you'll usually want to
// pass a fixed port for Spotify. Google's Desktop app OAuth client
// type accepts any loopback port, so port=0 is fine there.
//
// `successHTML` is shown in the user's browser after a successful
// callback; pass the empty string to use the default.
func Listen(port int, successHTML string) (*Loopback, error) {
	addrSpec := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addrSpec)
	if err != nil {
		return nil, fmt.Errorf("bind loopback %s: %w", addrSpec, err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	if successHTML == "" {
		successHTML = defaultSuccessHTML
	}

	lb := &Loopback{
		Port:   addr.Port,
		URL:    fmt.Sprintf("http://127.0.0.1:%d/callback", addr.Port),
		result: make(chan CallbackResult, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		res := CallbackResult{
			Code:  q.Get("code"),
			State: q.Get("state"),
		}
		if errStr := q.Get("error"); errStr != "" {
			res.Err = fmt.Errorf("provider returned error: %s (%s)", errStr, q.Get("error_description"))
		} else if res.Code == "" {
			res.Err = errors.New("callback missing code")
		}
		// Send result before writing response so if the provider closes
		// fast we still unblock the caller.
		select {
		case lb.result <- res:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if res.Err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
		_, _ = w.Write([]byte(renderHTML(successHTML, res.Err)))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	lb.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		_ = lb.server.Serve(ln)
	}()
	return lb, nil
}

// Wait blocks until the provider redirects to /callback or ctx
// cancels. Shuts down the server before returning regardless of
// outcome.
func (l *Loopback) Wait(ctx context.Context) CallbackResult {
	defer l.Shutdown()
	select {
	case r := <-l.result:
		return r
	case <-ctx.Done():
		return CallbackResult{Err: ctx.Err()}
	}
}

// Shutdown is idempotent.
func (l *Loopback) Shutdown() {
	if l.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = l.server.Shutdown(ctx)
	l.server = nil
}

// ─────────────────── PKCE helpers ───────────────────

// PKCE generates a (verifier, challenge) pair per RFC 7636. Both
// Google and Spotify accept S256 challenges; we only use S256.
func PKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

// SpotifyLoopbackPort is the fixed port the Spotify OAuth flow
// uses. It matches musicTUI's playback callback so users only need
// one Spotify redirect URI registered in their dev app:
// http://127.0.0.1:8888/callback
const SpotifyLoopbackPort = 8888

// GoogleLoopbackPort is the fixed port for Google OAuth. Used so
// the setup wizard can give exact instructions. Google's Desktop
// app type would actually accept any port, but a fixed value makes
// the wizard's "register this URI" guidance unambiguous.
const GoogleLoopbackPort = 8889

// State mints a random URL-safe nonce for CSRF protection.
func State() string {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		// Extremely unlikely; a zero-length state would be rejected anyway.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ─────────────────── rendered HTML ───────────────────

const defaultSuccessHTML = "Connected. You can close this tab."

func renderHTML(msg string, err error) string {
	color := "#6ce07c"
	title := "✓ Connected"
	body := msg
	if err != nil {
		color = "#fa586a"
		title = "✗ Authorization failed"
		body = err.Error()
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>musictui-import</title>
<style>
  html, body {
    margin: 0; height: 100%%;
    background: #0e0f13; color: #e3e6ed;
    font: 15px/1.5 system-ui, -apple-system, sans-serif;
    display: flex; align-items: center; justify-content: center;
  }
  .card {
    max-width: 26rem; padding: 2rem;
    background: #181a22; border: 1px solid #2a2d38;
    border-radius: 12px; text-align: center;
  }
  h1 { color: %s; font-size: 1.1rem; margin: 0 0 0.5rem; }
  p  { color: #8a92a6; margin: 0; }
</style>
</head>
<body>
  <div class="card">
    <h1>%s</h1>
    <p>%s</p>
  </div>
</body>
</html>`, color, title, htmlEscape(body))
}

func htmlEscape(s string) string {
	replacer := []struct{ o, n string }{
		{"&", "&amp;"},
		{"<", "&lt;"},
		{">", "&gt;"},
		{"\"", "&quot;"},
	}
	for _, r := range replacer {
		s = replaceAll(s, r.o, r.n)
	}
	return s
}

func replaceAll(s, old, new string) string {
	// Tiny dependency-free replace to avoid an import of "strings" just
	// for this one call. Not performance-sensitive.
	if old == "" {
		return s
	}
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}

// Used by tests that need to reach the loopback URL via a raw HTTP
// GET. Callers outside this package should not depend on this.
var _ = url.QueryEscape
