package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/iamteedoh/musicTUI/internal/importcore/cliconfig"
)

func runSetup(ctx context.Context, args []string) error {
	fs := flagSet("setup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = ctx

	cfg, err := cliconfig.Load()
	if err != nil {
		return err
	}

	fmt.Println(setupIntro)
	fmt.Println()

	fmt.Println("── Google Cloud (YouTube Music library read) ──")
	fmt.Println(googleInstructions)
	cfg.Google.ClientID = prompt("Google client_id", cfg.Google.ClientID)
	cfg.Google.ClientSecret = prompt("Google client_secret", cfg.Google.ClientSecret)

	fmt.Println()
	fmt.Println("── Spotify (destination) ──")
	fmt.Println(spotifyInstructions)
	cfg.Spotify.ClientID = prompt("Spotify client_id", cfg.Spotify.ClientID)
	cfg.Spotify.ClientSecret = prompt("Spotify client_secret", cfg.Spotify.ClientSecret)

	if err := cliconfig.Save(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	path, _ := cliconfig.Path()
	fmt.Printf("\n✓ Saved to %s\n", path)
	if missing := cfg.Missing(); len(missing) > 0 {
		fmt.Printf("\n⚠ Still missing: %s\n", strings.Join(missing, ", "))
	} else {
		fmt.Println("\nNext: musictui-import auth youtube")
		fmt.Println("      musictui-import auth spotify")
	}
	return nil
}

const setupIntro = `This wizard collects your OAuth client credentials so the
standalone CLI can read your YouTube library and write to your
Spotify library. Each service requires creating one OAuth app —
one-time work, after which imports are entirely local to your
machine.`

const googleInstructions = `  1. Go to https://console.cloud.google.com and create a new
     project (or reuse an existing one).
  2. Enable "YouTube Data API v3" under APIs & Services → Library.
  3. Configure OAuth consent screen: User type = External, status
     = Testing. Add your own Google account under Test Users.
  4. Create OAuth 2.0 Credentials → Application type = Web
     application. Authorised redirect URI: http://127.0.0.1
     (the loopback wildcard form Google allows for desktop apps).
  5. Copy the Client ID and Client Secret below.`

const spotifyInstructions = `  1. Go to https://developer.spotify.com/dashboard and create an
     app (or reuse your existing musicTUI app).
  2. Redirect URI: http://127.0.0.1:<port>/callback — Spotify
     requires an explicit port, so pick one and remember it, or
     let the CLI use a random port and re-run auth after adjusting.
     (The CLI tries 43117 by default; add that to your redirect list.)
  3. Under User Management, add your own Spotify account.
  4. Copy the Client ID and Client Secret below.`

// prompt reads a line from stdin. Shows the existing value in
// brackets if non-empty; pressing Enter keeps it.
func prompt(label, current string) string {
	reader := bufio.NewReader(os.Stdin)
	if current != "" {
		fmt.Printf("%s [%s]: ", label, maskSecret(current))
	} else {
		fmt.Printf("%s: ", label)
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return current
	}
	line = strings.TrimRight(line, "\r\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return current
	}
	return line
}

// maskSecret shows a hint at an existing credential without
// revealing it in full. Not security — just tidy output.
func maskSecret(s string) string {
	if len(s) < 6 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}
