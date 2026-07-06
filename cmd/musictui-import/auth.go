package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/iamteedoh/musicTUI/internal/importcore/cliconfig"
	"github.com/iamteedoh/musicTUI/internal/importcore/oauth"
	"github.com/iamteedoh/musicTUI/internal/importcore/store"
)

const authTimeout = 10 * time.Minute

func runAuth(ctx context.Context, args []string) error {
	fs := flagSet("auth")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: musictui-import auth <youtube|spotify>")
	}
	service := fs.Arg(0)

	cfg, err := cliconfig.Load()
	if err != nil {
		return err
	}
	if !cfg.IsReady() {
		return fmt.Errorf("run `musictui-import setup` first (missing: %v)", cfg.Missing())
	}

	tokStore, err := tokenStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	switch service {
	case "youtube":
		return authYouTube(ctx, cfg, tokStore)
	case "spotify":
		return authSpotify(ctx, cfg, tokStore)
	default:
		return fmt.Errorf("unknown service %q (use 'youtube' or 'spotify')", service)
	}
}

func authYouTube(ctx context.Context, cfg *cliconfig.Config, st *store.FileStore) error {
	lb, err := oauth.Listen(oauth.GoogleLoopbackPort, "")
	if err != nil {
		return err
	}
	verifier, challenge, err := oauth.PKCE()
	if err != nil {
		return err
	}
	state := oauth.State()
	gcfg := oauth.GoogleConfig{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
	}
	authURL := oauth.GoogleAuthorizeURL(gcfg, lb.URL, state, challenge)

	fmt.Println("Opening browser for YouTube sign-in…")
	fmt.Println("If the browser doesn't open, copy this URL manually:")
	fmt.Println(" ", authURL)
	_ = openBrowser(authURL)

	res := lb.Wait(ctx)
	if res.Err != nil {
		return res.Err
	}
	if res.State != state {
		return fmt.Errorf("state nonce mismatch — possible CSRF")
	}

	tok, err := oauth.GoogleExchangeCode(ctx, gcfg, lb.URL, res.Code, verifier)
	if err != nil {
		return err
	}
	if err := st.Save("youtube", &oauth.ServiceToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	}); err != nil {
		return err
	}
	fmt.Println("✓ YouTube connected.")
	return nil
}

func authSpotify(ctx context.Context, cfg *cliconfig.Config, st *store.FileStore) error {
	lb, err := oauth.Listen(oauth.SpotifyLoopbackPort, "")
	if err != nil {
		return err
	}
	state := oauth.State()
	scfg := oauth.SpotifyConfig{
		ClientID:     cfg.Spotify.ClientID,
		ClientSecret: cfg.Spotify.ClientSecret,
	}
	authURL := oauth.SpotifyAuthorizeURL(scfg, lb.URL, state)

	fmt.Println("Opening browser for Spotify sign-in…")
	fmt.Println("If the browser doesn't open, copy this URL manually:")
	fmt.Println(" ", authURL)
	fmt.Println()
	fmt.Printf("Note: Spotify requires the redirect URI in your dev app's\n")
	fmt.Printf("      settings. Add this exact URL:\n        %s\n", lb.URL)
	_ = openBrowser(authURL)

	res := lb.Wait(ctx)
	if res.Err != nil {
		return res.Err
	}
	if res.State != state {
		return fmt.Errorf("state nonce mismatch — possible CSRF")
	}

	tok, err := oauth.SpotifyExchangeCode(ctx, scfg, lb.URL, res.Code)
	if err != nil {
		return err
	}
	if err := st.Save("spotify", &oauth.ServiceToken{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	}); err != nil {
		return err
	}
	fmt.Println("✓ Spotify connected.")
	return nil
}

// tokenStore returns a FileStore rooted at the standalone CLI's
// import-tokens dir.
func tokenStore() (*store.FileStore, error) {
	dir, err := cliconfig.Dir()
	if err != nil {
		return nil, err
	}
	return store.NewFileStore(dir)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
