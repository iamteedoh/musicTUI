package components

import (
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

func TestOnboardStartAndNavigationBounds(t *testing.T) {
	o := NewOnboard()
	o.Start()

	if !o.Active {
		t.Fatal("Start() did not activate onboarding")
	}
	if o.Step != 0 {
		t.Fatalf("Step = %d, want 0", o.Step)
	}

	o.Prev()
	if o.Step != 0 {
		t.Fatalf("Prev() moved before first step: %d", o.Step)
	}

	for i := 0; i < TotalSteps+2; i++ {
		o.Next()
	}
	if o.Step != TotalSteps-1 {
		t.Fatalf("Next() moved past final step: %d", o.Step)
	}

	o.Close()
	if o.Active {
		t.Fatal("Close() left onboarding active")
	}
}

func TestOnboardStartAtClientIDPrefillsFinalStep(t *testing.T) {
	o := NewOnboard()
	o.StartAtClientID("existing_bad_id")

	if !o.Active {
		t.Fatal("StartAtClientID() did not activate onboarding")
	}
	if !o.OnFinalStep() {
		t.Fatalf("Step = %d, want final step %d", o.Step, TotalSteps-1)
	}
	if got := o.ClientID(); got != "existing_bad_id" {
		t.Fatalf("ClientID() = %q, want the pre-filled value", got)
	}
	// Cursor is at the end, so editing (backspace) acts on the pre-filled value.
	o.Backspace()
	if got := o.ClientID(); got != "existing_bad_i" {
		t.Fatalf("ClientID() after Backspace() = %q, want existing_bad_i", got)
	}
}

func TestOnboardOnlyAcceptsInputOnFinalStep(t *testing.T) {
	o := NewOnboard()
	o.Start()

	o.InputChar('a')
	if o.ClientIDInput != "" {
		t.Fatalf("ClientIDInput = %q before final step, want empty", o.ClientIDInput)
	}

	for !o.OnFinalStep() {
		o.Next()
	}
	o.InputChar('a')
	o.InputChar('b')
	o.InputChar('c')
	if got := o.ClientID(); got != "abc" {
		t.Fatalf("ClientID() = %q, want abc", got)
	}

	o.Backspace()
	if got := o.ClientID(); got != "ab" {
		t.Fatalf("ClientID() after Backspace() = %q, want ab", got)
	}
}

func TestImportSetupSecretFieldMasksValue(t *testing.T) {
	w := NewImportSetup()
	rendered := w.renderField("Client Secret", "super-secret", true, true, theme.Nord())

	if strings.Contains(rendered, "super-secret") {
		t.Fatalf("secret field rendered raw secret: %q", rendered)
	}
	if !strings.Contains(rendered, "Client Secret") {
		t.Fatalf("secret field did not render label: %q", rendered)
	}
}

func TestImportSetupPasteStripsControlCharacters(t *testing.T) {
	w := NewImportSetup()
	w.Start("", "", "", "")
	w.Step = 5

	w.Paste("abc\n\tdef")
	if w.GoogleClientID != "abcdef" {
		t.Fatalf("GoogleClientID = %q, want abcdef", w.GoogleClientID)
	}
}

// Re-running the wizard must NOT pre-fill secret inputs (a masked value the
// user can't read or replace made rotating a secret nearly impossible).
// Blank input keeps the saved secret; typing replaces it; ClearField empties.
func TestImportSetupSecretKeepReplaceAndClear(t *testing.T) {
	var w ImportSetup
	w.Start("gid", "old-google-secret", "", "old-spotify-secret")

	if w.GoogleClientSecret != "" || w.SpotifyClientSecret != "" {
		t.Fatalf("secret inputs must start empty on re-run, got %q / %q",
			w.GoogleClientSecret, w.SpotifyClientSecret)
	}

	// Blank input → saved values are kept.
	_, gSecret, _, sSecret := w.Trimmed()
	if gSecret != "old-google-secret" || sSecret != "old-spotify-secret" {
		t.Fatalf("blank inputs must keep saved secrets, got %q / %q", gSecret, sSecret)
	}
	if !w.Complete() {
		t.Fatal("wizard with saved secrets and blank inputs must count as complete")
	}

	// Typing a new secret replaces the saved one.
	w.Step = 8 // Spotify creds step (reuse path: one field)
	w.Paste("new-spotify-secret")
	_, _, _, sSecret = w.Trimmed()
	if sSecret != "new-spotify-secret" {
		t.Fatalf("typed secret must replace saved one, got %q", sSecret)
	}

	// ClearField empties the input → back to keeping the saved value.
	w.ClearField()
	if w.SpotifyClientSecret != "" || w.CursorPos != 0 {
		t.Fatalf("ClearField left %q (cursor %d)", w.SpotifyClientSecret, w.CursorPos)
	}
	_, _, _, sSecret = w.Trimmed()
	if sSecret != "old-spotify-secret" {
		t.Fatalf("after clear, blank must keep saved secret again, got %q", sSecret)
	}
}

// Ctrl+O recovery pre-fills the saved Client ID. A config written by an
// affected Windows build holds a U+0000 (MUS-23); reopening the wizard must
// clean it, otherwise "enter it again" re-submits the id Spotify already
// rejected. ClientID() strips controls as the last gate before we save.
func TestOnboardStripsControlCharsFromSavedAndTypedClientID(t *testing.T) {
	o := NewOnboard()
	o.StartAtClientID("\x00abc123")

	if o.ClientIDInput != "abc123" {
		t.Fatalf("prefill = %q, want abc123 — poisoned config was not healed", o.ClientIDInput)
	}
	if o.CursorPos != len("abc123") {
		t.Fatalf("CursorPos = %d, want %d", o.CursorPos, len("abc123"))
	}

	o.ClientIDInput = "\x00ab\x7fc\x1f"
	if got := o.ClientID(); got != "abc" {
		t.Fatalf("ClientID() = %q, want abc", got)
	}
}
