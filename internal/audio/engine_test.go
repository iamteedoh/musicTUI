package audio

import (
	"io"
	"os"
	"testing"
	"time"
)

// fakeBridgeEnv makes the test binary re-exec itself as a stand-in for
// player-bridge. Writing a #!/bin/sh script instead would be simpler, but
// Windows can't exec one, so the audio lifecycle tests never ran there.
const fakeBridgeEnv = "MUSICTUI_TEST_FAKE_BRIDGE"

func TestMain(m *testing.M) {
	if os.Getenv(fakeBridgeEnv) == "1" {
		// Stand in for player-bridge: consume stdin until EOF, emit nothing.
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeBridge returns the path to a minimal executable that consumes stdin
// until EOF — enough to stand in for player-bridge in lifecycle tests. The
// engine inherits our environment, so the child sees fakeBridgeEnv and takes
// the branch in TestMain above.
func fakeBridge(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	t.Setenv(fakeBridgeEnv, "1")
	return exe
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
