package components

import (
	"errors"
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/theme"
)

func TestImportErrorAdviceInvalidGrant(t *testing.T) {
	err := errors.New(`youtube token: google refresh: google token: 400: {"error":"invalid_grant"}`)

	advice := ImportErrorAdviceFor(err)
	if advice.Service != "youtube" {
		t.Fatalf("Service = %q, want youtube", advice.Service)
	}
	joined := strings.Join(advice.Lines, "\n")
	for _, want := range []string{"saved YouTube login", "Press r", "OAuth client"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("advice %q missing %q", joined, want)
		}
	}
}

func TestImportErrorAdviceInvalidClient(t *testing.T) {
	err := errors.New(`spotify auth: invalid_client`)

	advice := ImportErrorAdviceFor(err)
	if advice.Service != "spotify" {
		t.Fatalf("Service = %q, want spotify", advice.Service)
	}
	joined := strings.Join(advice.Lines, "\n")
	if !strings.Contains(joined, "client ID or client secret") {
		t.Fatalf("advice %q missing client guidance", joined)
	}
}

func TestImportErrorViewRendersActionableRecovery(t *testing.T) {
	importView := Import{
		Stage: ImportStageError,
		Err:   errors.New(`youtube token: google refresh: google token: 400: {"error":"invalid_grant"}`),
	}

	rendered := importView.viewError(theme.Nord(), 80)
	for _, want := range []string{
		"How to fix",
		"saved YouTube login",
		"r",
		"reconnect YouTube",
		"c",
		"reconfigure",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("viewError output missing %q:\n%s", want, rendered)
		}
	}
}
