package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// printableRune reports whether r may be inserted into a text input. It
// mirrors the rule ImportSetup.Paste already applies to pasted blobs: drop the
// C0 controls and DEL, keep everything else (including non-ASCII).
func printableRune(r rune) bool {
	return r >= 0x20 && r != 0x7f
}

// typedRunes returns the runes of a key event that represent characters the
// user actually typed.
//
// On Windows, bubbletea reads the console directly rather than an ANSI stream:
// it emits a KeyMsg for every key-down event except Shift, and its keyType()
// falls through to KeyRunes for any key that carries no character. A bare
// Ctrl, Alt, CapsLock or Win press therefore arrives as Runes: []rune{0}.
// U+0000 is not whitespace, so it survives strings.TrimSpace and silently
// corrupts the field — pressing Ctrl to paste a Spotify Client ID prefixed it
// with a NUL and Spotify answered "Invalid client id" (MUS-23). The ANSI
// reader used everywhere else never produces these events.
func typedRunes(msg tea.KeyMsg) []rune {
	out := make([]rune, 0, len(msg.Runes))
	for _, r := range msg.Runes {
		if printableRune(r) {
			out = append(out, r)
		}
	}
	return out
}
