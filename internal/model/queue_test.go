package model

import "testing"

func testTrack(id string) Track {
	return Track{ID: id, Name: "Track " + id}
}

func TestQueueStartsEmpty(t *testing.T) {
	q := NewQueue()

	if !q.IsEmpty() {
		t.Fatal("new queue is not empty")
	}
	if q.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", q.Len())
	}
	if q.Current() != nil {
		t.Fatal("Current() returned a track for an empty queue")
	}
}

func TestSetQueueCopiesTracksAndSelectsStartIndex(t *testing.T) {
	tracks := []Track{testTrack("a"), testTrack("b"), testTrack("c")}
	q := NewQueue()

	q.SetQueue(tracks, 1)
	tracks[1].Name = "mutated"

	if q.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", q.Len())
	}
	if got := q.Current(); got == nil || got.ID != "b" || got.Name == "mutated" {
		t.Fatalf("Current() = %#v, want copied track b", got)
	}
}

func TestSetQueueClampsTooLargeStartIndex(t *testing.T) {
	q := NewQueue()
	q.SetQueue([]Track{testTrack("a"), testTrack("b")}, 99)

	if got := q.Current(); got == nil || got.ID != "a" {
		t.Fatalf("Current() = %#v, want first track", got)
	}
}

func TestNextStopsAtEndWithoutRepeat(t *testing.T) {
	q := NewQueue()
	q.SetQueue([]Track{testTrack("a"), testTrack("b")}, 0)

	if got := q.Next(); got == nil || got.ID != "b" {
		t.Fatalf("first Next() = %#v, want b", got)
	}
	if got := q.Next(); got != nil {
		t.Fatalf("second Next() = %#v, want nil", got)
	}
	if got := q.Current(); got == nil || got.ID != "b" {
		t.Fatalf("Current() after end = %#v, want b", got)
	}
}

func TestNextRepeatsContext(t *testing.T) {
	q := NewQueue()
	q.SetQueue([]Track{testTrack("a"), testTrack("b")}, 1)
	q.Repeat = RepeatContext

	if got := q.Next(); got == nil || got.ID != "a" {
		t.Fatalf("Next() = %#v, want wrapped first track", got)
	}
}

func TestNextRepeatsTrack(t *testing.T) {
	q := NewQueue()
	q.SetQueue([]Track{testTrack("a"), testTrack("b")}, 0)
	q.Repeat = RepeatTrack

	if got := q.Next(); got == nil || got.ID != "a" {
		t.Fatalf("Next() = %#v, want same track", got)
	}
}

func TestPreviousBehavior(t *testing.T) {
	q := NewQueue()
	q.SetQueue([]Track{testTrack("a"), testTrack("b")}, 1)

	if got := q.Previous(); got == nil || got.ID != "a" {
		t.Fatalf("Previous() = %#v, want a", got)
	}
	if got := q.Previous(); got == nil || got.ID != "a" {
		t.Fatalf("Previous() at start = %#v, want a", got)
	}

	q.Repeat = RepeatContext
	if got := q.Previous(); got == nil || got.ID != "b" {
		t.Fatalf("Previous() with repeat context = %#v, want b", got)
	}
}

func TestRepeatModeCyclesAndFormats(t *testing.T) {
	if RepeatOff.String() != "Off" {
		t.Fatalf("RepeatOff.String() = %q", RepeatOff.String())
	}
	if RepeatOff.Next() != RepeatContext {
		t.Fatal("RepeatOff.Next() did not return RepeatContext")
	}
	if RepeatContext.Next() != RepeatTrack {
		t.Fatal("RepeatContext.Next() did not return RepeatTrack")
	}
	if RepeatTrack.Next() != RepeatOff {
		t.Fatal("RepeatTrack.Next() did not return RepeatOff")
	}
}
