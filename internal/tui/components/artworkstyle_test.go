package components

import "testing"

// clearArtworkEnv neutralizes every environment variable DetectArtworkStyle
// consults, so tests behave the same under CI and under `go test` run inside
// a real terminal (iTerm2 sets TERM_PROGRAM and LC_TERMINAL, Konsole sets
// KONSOLE_VERSION, and so on — exactly the signals under test).
func clearArtworkEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"MUSICTUI_ARTWORK", "TMUX",
		"KONSOLE_VERSION", "KONSOLE_DBUS_SESSION",
		"TERM_PROGRAM", "LC_TERMINAL",
		"KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("TERM", "xterm-256color") // deliberately uninformative
}

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
	clearArtworkEnv(t)
	t.Setenv("KONSOLE_VERSION", "260402")

	if got := DetectArtworkStyle(true, true); got != StyleSixel {
		t.Fatalf("Konsole (kitty=true, sixel=true) chose %v, want StyleSixel", got)
	}
	// A Konsole build without sixel must not fall back to kitty placeholders.
	if got := DetectArtworkStyle(true, false); got != StyleBlocks {
		t.Fatalf("Konsole (kitty=true, sixel=false) chose %v, want StyleBlocks", got)
	}
}

// iTerm2 is the other terminal that answers the kitty a=q query with ";OK"
// (since 3.6) without rendering Unicode placeholders — trusting the probe
// left a blank artwork panel (MUS-30). And unlike Konsole it cannot take the
// sixel path: it reports no usable cell size (its CSI 14 t answer includes
// window padding, so the 14/18 division is inexact). It must use its native
// OSC 1337 inline-image protocol, which is sized in cells and needs neither.
//
// These are the real capabilities iTerm2 reported (the MUS-30 diagnostic):
//
//	^[_Gi=4211;OK^[\ ^[[4;1290;2250t ^[[8;77;280t ^[]11;rgb:fffe/ffff/ffff^[\ ^[[?64;1;2;4;6;17;18;21;22;52c
func TestITerm2PrefersNativeInlineImages(t *testing.T) {
	clearArtworkEnv(t)
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	// kitty=true is what iTerm2 ≥ 3.6 actually answers; sixel=false is what
	// termcap derives once the unusable cell size disables DA1's attribute 4.
	if got := DetectArtworkStyle(true, false); got != StyleITerm2 {
		t.Fatalf("iTerm2 (kitty=true, sixel=false) chose %v, want StyleITerm2", got)
	}
	// Older iTerm2 answers no probe at all; the native protocol still works.
	if got := DetectArtworkStyle(false, false); got != StyleITerm2 {
		t.Fatalf("iTerm2 (kitty=false, sixel=false) chose %v, want StyleITerm2", got)
	}
}

// Over ssh, TERM_PROGRAM is not forwarded but LC_TERMINAL usually is (iTerm2
// sends it; most sshds accept LC_*). The remote end must still be recognized.
func TestITerm2DetectedOverSSH(t *testing.T) {
	clearArtworkEnv(t)
	t.Setenv("LC_TERMINAL", "iTerm2")

	if got := DetectArtworkStyle(true, false); got != StyleITerm2 {
		t.Fatalf("ssh-from-iTerm2 (LC_TERMINAL only) chose %v, want StyleITerm2", got)
	}
}

// ...but LC_TERMINAL is an LC_* variable, so it is INHERITED: launch Ghostty
// from an iTerm2 shell and Ghostty runs with LC_TERMINAL=iTerm2 set by a
// terminal that is no longer on the other end of the tty. TERM_PROGRAM is set
// by the terminal that IS, so it has to win — OR-ing the two sent Ghostty the
// iTerm2 protocol, which it ignores, and the artwork panel went blank.
func TestGhosttyLaunchedFromITerm2StillPicksKitty(t *testing.T) {
	clearArtworkEnv(t)
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("LC_TERMINAL", "iTerm2") // stale, inherited from the parent shell

	if got := DetectArtworkStyle(true, false); got != StyleKitty {
		t.Fatalf("Ghostty with an inherited LC_TERMINAL chose %v, want StyleKitty", got)
	}
	// The same inheritance must not cost a kitty terminal its placeholders.
	if !kittyPlaceholders() {
		t.Fatal("kittyPlaceholders() denied a non-iTerm2 terminal on a stale LC_TERMINAL")
	}
}

// tmux consumes the OSC payload like every other graphics escape, so the tmux
// guard must keep winning over the iTerm2 identity check.
func TestITerm2InsideTmuxStillCharacterArt(t *testing.T) {
	clearArtworkEnv(t)
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")

	if got := DetectArtworkStyle(true, true); got != StyleBlocks {
		t.Fatalf("iTerm2 inside tmux chose %v, want StyleBlocks", got)
	}
}

// Ghostty and kitty implement placeholders, so a positive probe must still win
// there — including on Linux, where env sniffing alone misidentifies Ghostty
// (MUS-20). The probe, not the environment, is the signal.
func TestKittyPlaceholderTerminalsStillPickKitty(t *testing.T) {
	clearArtworkEnv(t)

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
	clearArtworkEnv(t)
	t.Setenv("KONSOLE_VERSION", "260402")
	t.Setenv("MUSICTUI_ARTWORK", "kitty")
	if got := DetectArtworkStyle(false, false); got != StyleKitty {
		t.Fatalf("MUSICTUI_ARTWORK=kitty chose %v", got)
	}
	t.Setenv("MUSICTUI_ARTWORK", "blocks")
	if got := DetectArtworkStyle(true, true); got != StyleBlocks {
		t.Fatalf("MUSICTUI_ARTWORK=blocks chose %v", got)
	}
	// iterm2 can be forced anywhere, and kitty can be forced back on inside
	// iTerm2 (the escape hatch if a future iTerm2 gains placeholder support).
	t.Setenv("MUSICTUI_ARTWORK", "iterm2")
	if got := DetectArtworkStyle(false, false); got != StyleITerm2 {
		t.Fatalf("MUSICTUI_ARTWORK=iterm2 chose %v", got)
	}
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("MUSICTUI_ARTWORK", "kitty")
	if got := DetectArtworkStyle(true, false); got != StyleKitty {
		t.Fatalf("MUSICTUI_ARTWORK=kitty inside iTerm2 chose %v", got)
	}
}

// tmux needs passthrough wrapping for both APC and DCS payloads.
func TestTmuxAlwaysCharacterArt(t *testing.T) {
	clearArtworkEnv(t)
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	if got := DetectArtworkStyle(true, true); got != StyleBlocks {
		t.Fatalf("inside tmux chose %v, want StyleBlocks", got)
	}
}
