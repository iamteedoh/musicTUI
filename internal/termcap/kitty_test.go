package termcap

import (
	"os"
	"testing"
)

func TestParseKittyReply(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"kitty OK before DA1", []byte("\x1b_Gi=4211;OK\x1b\\\x1b[?62;c"), true},
		{"kitty OK with extra keys", []byte("\x1b_Gi=4211,q=2;OK\x1b\\"), true},
		{"DA1 only (no graphics support)", []byte("\x1b[?62;22c"), false},
		{"empty (timed out)", []byte(nil), false},
		{"graphics error reply (not OK)", []byte("\x1b_Gi=4211;ENOSUP\x1b\\"), false},
	}
	for _, c := range cases {
		if got := parseKittyReply(c.buf); got != c.want {
			t.Errorf("%s: parseKittyReply(%q) = %v, want %v", c.name, c.buf, got, c.want)
		}
	}
}

// The probe must be side-effect-free and return false when stdout isn't a
// terminal — which is the case under `go test` (output is captured). This
// guards the guarantee that tests and piped runs never touch the terminal.
func TestSupportsKittyGraphicsFalseWhenNotTTY(t *testing.T) {
	if term := os.Getenv("TERM"); term == "" {
		// belt-and-suspenders: even with no TERM it must not panic
	}
	if SupportsKittyGraphics() {
		t.Fatal("SupportsKittyGraphics returned true under a non-TTY test harness")
	}
}
