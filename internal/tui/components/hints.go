package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/iamteedoh/musicTUI/internal/theme"
)

// Hint is a single key/description pair rendered in an inline hint row.
type Hint struct {
	Key  string
	Desc string
}

// RenderHints formats a sequence of hints as a single muted line using the
// app-wide convention — accent-colored keys, dim descriptions, middle-dot
// separators. Shared so every view's hint row stays visually consistent.
//
// Example output (with styling):
//
//	j/k: move  ·  Enter: play  ·  a: add to playlist
func RenderHints(th theme.Theme, hints []Hint) string {
	if len(hints) == 0 {
		return ""
	}
	keyStyle := lipgloss.NewStyle().Foreground(th.Accent).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(th.FgDim)
	sepStyle := lipgloss.NewStyle().Foreground(th.FgMuted)

	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		parts = append(parts, keyStyle.Render(h.Key)+descStyle.Render(": "+h.Desc))
	}
	return strings.Join(parts, sepStyle.Render("  ·  "))
}
