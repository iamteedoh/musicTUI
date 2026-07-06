package components

import (
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

// The waiting-for-login screen must tell the user how to recover from a
// wrong/rejected Spotify Client ID (MUS-12): the browser shows "Invalid
// client id" and the app needs to point at the in-app Ctrl+O fix.
func TestHomeWaitingStateShowsClientIDRecovery(t *testing.T) {
	th := theme.FromName("")
	h := NewHome()
	h.AuthURL = "https://accounts.spotify.com/authorize?client_id=bad"

	out := h.View(th, 80, 40)

	if !strings.Contains(out, "Ctrl+O") {
		t.Fatalf("waiting-state Home view does not mention Ctrl+O recovery:\n%s", out)
	}
	if !strings.Contains(out, "Invalid client id") {
		t.Fatalf("waiting-state Home view does not name the 'Invalid client id' symptom:\n%s", out)
	}
}
