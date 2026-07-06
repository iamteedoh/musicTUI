package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// The confirm modal must word-wrap its message to the box width (no line
// overflowing the frame) and center the key legend underneath.
func TestConfirmModalWrapsAndCentersLegend(t *testing.T) {
	th := theme.FromName("")
	var m Modal
	msg := "Remove them from your library? These will be unfollowed " +
		"(not deleted). A backup is saved first — press R afterwards to " +
		"restore, or use Spotify's 90-day recovery page."
	m.ShowConfirm("Remove Empty Playlists", msg, ActionDeleteEmptyPlaylists, "")

	out := m.View(th, 120, 40)

	// No rendered line may exceed the box width (guards the mid-word wrap bug).
	boxW := lipgloss.Width(out)
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > boxW {
			t.Fatalf("line wider than box (%d > %d): %q", w, boxW, stripAnsi(line))
		}
	}

	// The legend line must be centered: it carries the keys and has leading
	// padding rather than butting against the left border.
	var legend string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(stripAnsi(line), "yes") && strings.Contains(stripAnsi(line), "cancel") {
			legend = stripAnsi(line)
			break
		}
	}
	if legend == "" {
		t.Fatal("legend row (y: yes … Esc: cancel) not found in modal render")
	}
	// Strip only the border glyphs (not spaces), then require several leading
	// spaces before the keys — more than the box's single-space padding, which
	// is what distinguishes a centered legend from a left-aligned one.
	inner := strings.TrimLeft(legend, "│╭╮╰╯─")
	if leadSpaces := len(inner) - len(strings.TrimLeft(inner, " ")); leadSpaces < 3 {
		t.Fatalf("legend does not appear centered (only %d leading spaces): %q", leadSpaces, legend)
	}
}
