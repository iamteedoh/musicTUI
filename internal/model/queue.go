package model

import "math/rand"

// Queue manages an ordered list of tracks for playback.
type Queue struct {
	Tracks  []Track
	Index   int // currently playing index, -1 = none
	Shuffle bool
	Repeat  RepeatMode
}

// NewQueue creates an empty queue.
func NewQueue() Queue {
	return Queue{Index: -1}
}

// SetQueue replaces the queue with a new track list, starting at startIdx.
func (q *Queue) SetQueue(tracks []Track, startIdx int) {
	q.Tracks = make([]Track, len(tracks))
	copy(q.Tracks, tracks)
	q.Index = startIdx
	if q.Index >= len(q.Tracks) {
		q.Index = 0
	}
}

// Current returns the currently playing track, or nil if none.
func (q *Queue) Current() *Track {
	if q.Index >= 0 && q.Index < len(q.Tracks) {
		return &q.Tracks[q.Index]
	}
	return nil
}

// Next advances to the next track and returns it.
// Returns nil if at the end with no repeat.
func (q *Queue) Next() *Track {
	if len(q.Tracks) == 0 {
		return nil
	}

	if q.Repeat == RepeatTrack {
		return q.Current()
	}

	if q.Shuffle {
		q.Index = rand.Intn(len(q.Tracks))
		return q.Current()
	}

	q.Index++
	if q.Index >= len(q.Tracks) {
		if q.Repeat == RepeatContext {
			q.Index = 0
		} else {
			q.Index = len(q.Tracks) - 1
			return nil // end of queue
		}
	}
	return q.Current()
}

// Previous goes back one track and returns it.
func (q *Queue) Previous() *Track {
	if len(q.Tracks) == 0 {
		return nil
	}

	q.Index--
	if q.Index < 0 {
		if q.Repeat == RepeatContext {
			q.Index = len(q.Tracks) - 1
		} else {
			q.Index = 0
		}
	}
	return q.Current()
}

// IsEmpty returns true if the queue has no tracks.
func (q *Queue) IsEmpty() bool {
	return len(q.Tracks) == 0
}

// Len returns the number of tracks in the queue.
func (q *Queue) Len() int {
	return len(q.Tracks)
}
