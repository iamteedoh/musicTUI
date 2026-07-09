//go:build !unix && !windows

package termcap

// Detect is a no-op on platforms with neither a POSIX terminal nor a Windows
// console (e.g. js/wasm, plan9). Detection falls back to environment
// heuristics there, which means character art.
func Detect() Caps { return Caps{} }
