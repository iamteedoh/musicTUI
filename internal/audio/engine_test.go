package audio

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeBridge writes a minimal executable that consumes stdin until EOF —
// enough to stand in for player-bridge in lifecycle tests.
func fakeBridge(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-bridge")
	script := "#!/bin/sh\nwhile read line; do :; done\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// After the bridge process dies (librespot crash), the next PlayTrack must
// transparently restart it instead of failing with "bridge not running" on
// the track change (MUS-17).
func TestPlayTrackRecoversAfterBridgeDeath(t *testing.T) {
	e := NewEngine(fakeBridge(t), "tok")
	defer e.Close()

	if err := e.PlayTrack("track1"); err != nil {
		t.Fatalf("first PlayTrack: %v", err)
	}

	// Simulate a crash.
	e.mu.Lock()
	e.cmd.Process.Kill()
	e.mu.Unlock()

	// Whether or not the cleanup goroutine has run yet, the next PlayTrack
	// must succeed (dead-pipe write → restart → retry).
	deadline := time.Now().Add(5 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = e.PlayTrack("track2"); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("PlayTrack after bridge death did not recover: %v", err)
	}
}

// SetToken kills the bridge for a fresh session. A PlayTrack landing
// immediately after must succeed, and the OLD bridge's cleanup goroutine
// must not clobber the NEW bridge's state when it finally runs (MUS-17
// identity-guard race).
func TestSetTokenThenImmediatePlay(t *testing.T) {
	e := NewEngine(fakeBridge(t), "tok")
	defer e.Close()

	if err := e.PlayTrack("track1"); err != nil {
		t.Fatalf("first PlayTrack: %v", err)
	}

	e.SetToken("newtok")
	if err := e.PlayTrack("track2"); err != nil {
		t.Fatalf("PlayTrack immediately after SetToken: %v", err)
	}
	if !e.Started() {
		t.Fatal("engine not started after SetToken+PlayTrack")
	}

	// Give the old bridge's readEvents goroutine time to observe the kill
	// and run its exit cleanup — it must NOT mark the new bridge stopped.
	time.Sleep(500 * time.Millisecond)
	if !e.Started() {
		t.Fatal("old bridge's cleanup goroutine clobbered the new bridge's state (identity-guard regression)")
	}
}
