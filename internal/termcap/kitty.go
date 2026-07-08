// Package termcap detects terminal capabilities by querying the terminal
// directly rather than guessing from environment variables.
package termcap

import "bytes"

// queryID is the arbitrary image id used in the kitty graphics support query;
// the terminal echoes it back in its reply.
const queryID = 4211

// parseKittyReply reports whether buf contains kitty's positive graphics-support
// reply: an APC "G" response carrying ";OK". Terminals that don't implement the
// kitty graphics protocol ignore the query entirely and never emit an APC-G, so
// this is an unambiguous yes/no — unlike sniffing $TERM / $TERM_PROGRAM, which
// misidentifies Ghostty on Linux (MUS-20).
func parseKittyReply(buf []byte) bool {
	return bytes.Contains(buf, []byte("\x1b_G")) && bytes.Contains(buf, []byte(";OK"))
}
