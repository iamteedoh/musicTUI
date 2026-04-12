//go:build linux

package mpris

import (
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
)

// MediaCommand represents a media key action.
type MediaCommand int

const (
	CmdPlayPause MediaCommand = iota
	CmdNext
	CmdPrevious
	CmdStop
)

const (
	busName       = "org.mpris.MediaPlayer2.musicTUI"
	objectPath    = "/org/mpris/MediaPlayer2"
	playerIface   = "org.mpris.MediaPlayer2.Player"
	rootIface     = "org.mpris.MediaPlayer2"
	propsIface    = "org.freedesktop.DBus.Properties"
)

// Server registers musicTUI as an MPRIS media player on the session D-Bus.
// Media key presses from the desktop environment are sent to the Commands channel.
type Server struct {
	conn     *dbus.Conn
	commands chan MediaCommand
	mu       sync.Mutex
	closed   bool
}

// New creates and starts an MPRIS server. Returns nil if D-Bus is unavailable.
func New() *Server {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil
	}

	s := &Server{
		conn:     conn,
		commands: make(chan MediaCommand, 16),
	}

	// Export the MPRIS interfaces
	conn.Export(s, objectPath, rootIface)
	conn.Export(s, objectPath, playerIface)
	conn.Export(s, objectPath, propsIface)

	// Export introspection so D-Bus tools can discover us
	conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: objectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name: rootIface,
				Methods: []introspect.Method{
					{Name: "Quit"},
					{Name: "Raise"},
				},
				Properties: []introspect.Property{
					{Name: "CanQuit", Type: "b", Access: "read"},
					{Name: "CanRaise", Type: "b", Access: "read"},
					{Name: "HasTrackList", Type: "b", Access: "read"},
					{Name: "Identity", Type: "s", Access: "read"},
				},
			},
			{
				Name: playerIface,
				Methods: []introspect.Method{
					{Name: "PlayPause"},
					{Name: "Next"},
					{Name: "Previous"},
					{Name: "Stop"},
					{Name: "Play"},
					{Name: "Pause"},
				},
				Properties: []introspect.Property{
					{Name: "CanGoNext", Type: "b", Access: "read"},
					{Name: "CanGoPrevious", Type: "b", Access: "read"},
					{Name: "CanPlay", Type: "b", Access: "read"},
					{Name: "CanPause", Type: "b", Access: "read"},
					{Name: "CanControl", Type: "b", Access: "read"},
					{Name: "PlaybackStatus", Type: "s", Access: "read"},
				},
			},
		},
	}), objectPath, "org.freedesktop.DBus.Introspectable")

	// Request the bus name
	reply, err := conn.RequestName(busName, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil
	}

	return s
}

// Commands returns the channel that receives media key commands.
func (s *Server) Commands() <-chan MediaCommand {
	return s.commands
}

// Close shuts down the MPRIS server.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.commands)
		s.conn.Close()
	}
}

func (s *Server) send(cmd MediaCommand) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		select {
		case s.commands <- cmd:
		default:
		}
	}
}

// org.mpris.MediaPlayer2 methods
func (s *Server) Raise() *dbus.Error { return nil }
func (s *Server) Quit() *dbus.Error  { return nil }

// org.mpris.MediaPlayer2.Player methods
func (s *Server) PlayPause() *dbus.Error {
	s.send(CmdPlayPause)
	return nil
}

func (s *Server) Play() *dbus.Error {
	s.send(CmdPlayPause)
	return nil
}

func (s *Server) Pause() *dbus.Error {
	s.send(CmdPlayPause)
	return nil
}

func (s *Server) Next() *dbus.Error {
	s.send(CmdNext)
	return nil
}

func (s *Server) Previous() *dbus.Error {
	s.send(CmdPrevious)
	return nil
}

func (s *Server) Stop() *dbus.Error {
	s.send(CmdStop)
	return nil
}

// org.freedesktop.DBus.Properties
func (s *Server) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	switch iface {
	case rootIface:
		switch prop {
		case "CanQuit":
			return dbus.MakeVariant(false), nil
		case "CanRaise":
			return dbus.MakeVariant(false), nil
		case "HasTrackList":
			return dbus.MakeVariant(false), nil
		case "Identity":
			return dbus.MakeVariant("musicTUI"), nil
		}
	case playerIface:
		switch prop {
		case "CanGoNext":
			return dbus.MakeVariant(true), nil
		case "CanGoPrevious":
			return dbus.MakeVariant(true), nil
		case "CanPlay":
			return dbus.MakeVariant(true), nil
		case "CanPause":
			return dbus.MakeVariant(true), nil
		case "CanControl":
			return dbus.MakeVariant(true), nil
		case "PlaybackStatus":
			return dbus.MakeVariant("Playing"), nil
		}
	}
	return dbus.MakeVariant(""), nil
}

func (s *Server) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	props := make(map[string]dbus.Variant)
	switch iface {
	case rootIface:
		props["CanQuit"] = dbus.MakeVariant(false)
		props["CanRaise"] = dbus.MakeVariant(false)
		props["HasTrackList"] = dbus.MakeVariant(false)
		props["Identity"] = dbus.MakeVariant("musicTUI")
	case playerIface:
		props["CanGoNext"] = dbus.MakeVariant(true)
		props["CanGoPrevious"] = dbus.MakeVariant(true)
		props["CanPlay"] = dbus.MakeVariant(true)
		props["CanPause"] = dbus.MakeVariant(true)
		props["CanControl"] = dbus.MakeVariant(true)
		props["PlaybackStatus"] = dbus.MakeVariant("Playing")
	}
	return props, nil
}

func (s *Server) Set(iface, prop string, value dbus.Variant) *dbus.Error {
	return nil
}
