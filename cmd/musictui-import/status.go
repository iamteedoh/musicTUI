package main

import (
	"context"
	"fmt"
	"time"

	"github.com/iamteedoh/musicTUI/internal/importcore/cliconfig"
)

func runStatus(ctx context.Context, args []string) error {
	fs := flagSet("status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_ = ctx

	cfg, err := cliconfig.Load()
	if err != nil {
		return err
	}

	fmt.Println("Configuration:")
	path, _ := cliconfig.Path()
	fmt.Printf("  config file:       %s\n", path)
	printSet("google.client_id", cfg.Google.ClientID != "")
	printSet("google.client_secret", cfg.Google.ClientSecret != "")
	printSet("spotify.client_id", cfg.Spotify.ClientID != "")
	printSet("spotify.client_secret", cfg.Spotify.ClientSecret != "")

	fmt.Println("\nConnected services:")
	st, err := tokenStore()
	if err != nil {
		return err
	}
	for _, svc := range []string{"youtube", "spotify"} {
		tok, err := st.Load(svc)
		if err != nil {
			fmt.Printf("  %-8s error: %v\n", svc, err)
			continue
		}
		if tok == nil {
			fmt.Printf("  %-8s not connected — run `musictui-import auth %s`\n", svc, svc)
			continue
		}
		expiry := "unknown"
		if !tok.ExpiresAt.IsZero() {
			until := time.Until(tok.ExpiresAt)
			if until <= 0 {
				expiry = "expired (will auto-refresh on next call)"
			} else {
				expiry = tok.ExpiresAt.Format(time.RFC1123) +
					fmt.Sprintf(" (in %s)", truncateDuration(until))
			}
		}
		fmt.Printf("  %-8s connected — expires %s\n", svc, expiry)
	}
	return nil
}

func printSet(label string, ok bool) {
	state := "✗ missing"
	if ok {
		state = "✓ set"
	}
	fmt.Printf("  %-24s %s\n", label, state)
}

func truncateDuration(d time.Duration) string {
	// Display "1h30m" / "45m" / "55s" — not to the nanosecond.
	d = d.Round(time.Second)
	return d.String()
}
