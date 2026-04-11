package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}

func centerLn(text string, width int) string {
	w := lipgloss.Width(text)
	pad := (width - w) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + text
}

func cText(msg string, w, h int) string {
	mw := lipgloss.Width(msg)
	pad := (w - mw) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat("\n", h/2) + strings.Repeat(" ", pad) + msg
}
