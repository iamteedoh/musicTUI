//go:build !linux

package mpris

// MediaCommand represents a media key action.
type MediaCommand int

const (
	CmdPlayPause MediaCommand = iota
	CmdNext
	CmdPrevious
	CmdStop
)

// Server is a no-op on non-Linux platforms (no D-Bus).
type Server struct {
	commands chan MediaCommand
}

// New returns nil on non-Linux platforms.
func New() *Server { return nil }

// Commands returns nil on non-Linux platforms.
func (s *Server) Commands() <-chan MediaCommand { return nil }

// Close is a no-op.
func (s *Server) Close() {}
