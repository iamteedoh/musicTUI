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
