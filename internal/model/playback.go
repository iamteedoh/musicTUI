package model

import "time"

type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatContext
	RepeatTrack
)

func (r RepeatMode) String() string {
	switch r {
	case RepeatContext:
		return "Context"
	case RepeatTrack:
		return "Track"
	default:
		return "Off"
	}
}

func (r RepeatMode) Next() RepeatMode {
	return (r + 1) % 3
}

type PlaybackState struct {
	Track     *Track
	IsPlaying bool
	Position  time.Duration
	Volume    int // 0-100
	Shuffle   bool
	Repeat    RepeatMode
}
