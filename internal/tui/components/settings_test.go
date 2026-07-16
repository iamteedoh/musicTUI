package components

import (
	"strings"
	"testing"

	"github.com/iamteedoh/musicTUI/internal/config"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// The Theme row must show what Auto actually resolved to — a user on a light
// terminal needs to see the detection worked before deciding to override it —
// and an explicit choice shows its name and tier (MUS-32).
func TestSettingsViewShowsThemeValue(t *testing.T) {
	s := NewSettings()

	autoView := s.View(theme.Nord(), config.Config{Theme: "auto"}, 80, 24)
	if !strings.Contains(autoView, "Auto — Nord") {
		t.Fatalf("auto row missing resolved theme name:\n%s", autoView)
	}

	explicitView := s.View(theme.GruvboxLight(), config.Config{Theme: "gruvbox_light"}, 80, 24)
	if !strings.Contains(explicitView, "Gruvbox Light (light)") {
		t.Fatalf("explicit row missing name and tier:\n%s", explicitView)
	}
}

// Selection must stay within the settings list as rows are added or removed.
func TestSettingsNavigationBounds(t *testing.T) {
	s := NewSettings()
	if s.SelectedKey() != "theme" {
		t.Fatalf("first row = %q, want theme", s.SelectedKey())
	}
	s.Up() // already at the top
	if s.Selected != 0 {
		t.Fatalf("Up at top moved to %d", s.Selected)
	}
	for i := 0; i < 50; i++ {
		s.Down()
	}
	if s.Selected != len(settingItems)-1 {
		t.Fatalf("Down past bottom = %d, want %d", s.Selected, len(settingItems)-1)
	}
}
