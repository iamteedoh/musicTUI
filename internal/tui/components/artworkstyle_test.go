package components

import "testing"

// Konsole answers the kitty a=q support query with ";OK" because it implements
// kitty image *transmission* — but it has no Unicode-placeholder support, which
// is what renderPlaceholders needs. Choosing kitty there paints a crisp cover at
// the cursor, orphaned and duplicated on every resize. It must take the sixel
// path, which Konsole does implement (MUS-29).
//
// These are the real capabilities Konsole 26.04 reported on theo:
//
//	^[_Gi=4211;OK^[\ ^[[6;15;8t ^[[4;1320;2248t ^[[8;88;281t ^[[?62;1;4c
func TestKonsolePrefersSixelOverKittyPlaceholders(t *testing.T) {
	t.Setenv("KONSOLE_VERSION", "260402")
	t.Setenv("TERM", "xterm-256color")

	if got := DetectArtworkStyle(true, true); got != StyleSixel {
		t.Fatalf("Konsole (kitty=true, sixel=true) chose %v, want StyleSixel", got)
	}
	// A Konsole build without sixel must not fall back to kitty placeholders.
	if got := DetectArtworkStyle(true, false); got != StyleBlocks {
		t.Fatalf("Konsole (kitty=true, sixel=false) chose %v, want StyleBlocks", got)
	}
}

// Ghostty and kitty implement placeholders, so a positive probe must still win
// there — including on Linux, where env sniffing alone misidentifies Ghostty
// (MUS-20). The probe, not the environment, is the signal.
func TestKittyPlaceholderTerminalsStillPickKitty(t *testing.T) {
	t.Setenv("TERM", "xterm-256color") // deliberately uninformative
	t.Setenv("TERM_PROGRAM", "")

	if got := DetectArtworkStyle(true, false); got != StyleKitty {
		t.Fatalf("kitty-capable terminal chose %v, want StyleKitty", got)
	}
	// Kitty graphics beats sixel when both are on offer.
	if got := DetectArtworkStyle(true, true); got != StyleKitty {
		t.Fatalf("kitty+sixel terminal chose %v, want StyleKitty", got)
	}
}

// The env override wins over everything, so a user can escape a bad guess.
func TestArtworkEnvOverrideWins(t *testing.T) {
	t.Setenv("KONSOLE_VERSION", "260402")
	t.Setenv("MUSICTUI_ARTWORK", "kitty")
	if got := DetectArtworkStyle(false, false); got != StyleKitty {
		t.Fatalf("MUSICTUI_ARTWORK=kitty chose %v", got)
	}
	t.Setenv("MUSICTUI_ARTWORK", "blocks")
	if got := DetectArtworkStyle(true, true); got != StyleBlocks {
		t.Fatalf("MUSICTUI_ARTWORK=blocks chose %v", got)
	}
}

// tmux needs passthrough wrapping for both APC and DCS payloads.
func TestTmuxAlwaysCharacterArt(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if got := DetectArtworkStyle(true, true); got != StyleBlocks {
		t.Fatalf("inside tmux chose %v, want StyleBlocks", got)
	}
}
