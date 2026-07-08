//go:build !unix

package termcap

// SupportsKittyGraphics is a no-op on platforms without a POSIX terminal probe
// (e.g. Windows). Detection falls back to environment heuristics there — kitty
// and Ghostty do not target those platforms today.
func SupportsKittyGraphics() bool { return false }
