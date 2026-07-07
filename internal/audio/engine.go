package audio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// bridgeCommand is sent to the Rust player-bridge via stdin.
type bridgeCommand struct {
	Cmd        string `json:"cmd"`
	Token      string `json:"token,omitempty"`
	TrackID    string `json:"track_id,omitempty"`
	PositionMs uint32 `json:"position_ms,omitempty"`
	Value      uint8  `json:"value,omitempty"`
}

// bridgeEvent is received from the Rust player-bridge via stderr.
type bridgeEvent struct {
	Event         string    `json:"event"`
	TrackID       string    `json:"track_id,omitempty"`
	PositionMs    *int64    `json:"position_ms,omitempty"`
	Message       string    `json:"message,omitempty"`
	Magnitudes    []float32 `json:"magnitudes,omitempty"`
	Bass          *float32  `json:"bass,omitempty"`
	Mids          *float32  `json:"mids,omitempty"`
	Highs         *float32  `json:"highs,omitempty"`
	Energy        *float32  `json:"energy,omitempty"`
	Beat          *bool     `json:"beat,omitempty"`
	BeatIntensity *float32  `json:"beat_intensity,omitempty"`
	Bpm           *float32  `json:"bpm,omitempty"`
}

// Event is sent from the engine to the TUI.
type Event struct {
	Kind       string // "playing", "paused", "stopped", "loading", "error", "position", "end_of_track"
	PositionMs int64
	Error      string
	TrackID    string
}

// Engine manages the Rust player-bridge subprocess.
type Engine struct {
	bridgePath string
	token      string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	events     chan Event
	positionMs atomic.Int64
	volume     atomic.Int32
	playing    atomic.Bool
	started    bool
	mu         sync.Mutex

	// Debug log — captures stderr lines (librespot logs + non-spectrum JSON
	// events). Helps diagnose platform-specific audio/auth issues that would
	// otherwise be invisible behind the alt-screen TUI. Size-capped so a
	// misbehaving track (e.g. symphonia spamming decode warnings) can't grow
	// the file without bound — see writeLog. Only touched by readEvents'
	// goroutine, so logBytes/logCapped need no locking.
	logFile   *os.File
	logBytes  int64
	logCapped bool

	// Spectrum analysis. Spectrum is populated from the Rust bridge's FFT
	// thread via the "spectrum" events in readEvents — there is no Go-side FFT.
	Spectrum *SharedSpectrum
}

// LogPath returns the path where bridge stderr is captured.
func LogPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "musicTUI", "bridge.log")
}

// NewEngine creates a new audio engine. The bridge subprocess is NOT started
// until the first PlayTrack call — this avoids interfering with the terminal.
func NewEngine(bridgePath, token string) *Engine {
	spectrum := NewSharedSpectrum()
	e := &Engine{
		bridgePath: bridgePath,
		token:      token,
		events:     make(chan Event, 64),
		Spectrum:   spectrum,
	}
	e.volume.Store(75)
	return e
}

func (e *Engine) ensureBridge() error {
	if e.started {
		return nil
	}

	// Open (or re-open) the bridge debug log. Append-mode so we keep
	// history across restarts within a single TUI session; truncated
	// on the first open below.
	if e.logFile == nil {
		path := LogPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			// Truncate on first open so each run starts with a clean slate.
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err == nil {
				fmt.Fprintf(f, "=== musicTUI bridge log — %s ===\n", time.Now().Format(time.RFC3339))
				e.logFile = f
			}
		}
	}

	e.cmd = exec.Command(e.bridgePath)
	setSysProcAttr(e.cmd)

	// Ask librespot for info-level logs so the bridge.log file has enough
	// detail to diagnose auth/device failures, but silence symphonia's
	// per-frame demuxer warnings ("skipping junk", "invalid mpeg audio
	// header") which a malformed/undecodable track emits thousands of times
	// per second. Callers can override by exporting RUST_LOG before launch.
	if os.Getenv("RUST_LOG") == "" {
		e.cmd.Env = append(os.Environ(), "RUST_LOG=librespot=info,symphonia=error,info")
	}

	var err error
	e.stdin, err = e.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stderr, err := e.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	// Discard stdout (bridge handles audio output directly via librespot)
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("open devnull: %w", err)
	}
	e.cmd.Stdout = devnull

	if err := e.cmd.Start(); err != nil {
		devnull.Close()
		return fmt.Errorf("start player-bridge: %w", err)
	}
	devnull.Close()

	e.started = true
	go e.readEvents(stderr, e.cmd)

	return nil
}

// maxLogBytes caps the bridge debug log per TUI session. Plenty for
// diagnostics; small enough that a runaway track can't fill the disk. The log
// is truncated fresh on each launch (see ensureBridge), so this is a per-run
// ceiling, not cumulative across runs.
const maxLogBytes = 8 << 20 // 8 MiB

// writeLog appends a line to the bridge debug log, enforcing maxLogBytes. Once
// the cap is hit it writes a single truncation marker and stops, so a
// misbehaving bridge can't grow the file without bound. Called only from
// readEvents' goroutine.
func (e *Engine) writeLog(line string) {
	if e.logFile == nil || e.logCapped {
		return
	}
	n, _ := fmt.Fprintln(e.logFile, line)
	e.logBytes += int64(n)
	if e.logBytes >= maxLogBytes {
		fmt.Fprintf(e.logFile, "=== log capped at %d bytes — further bridge output suppressed for this session ===\n", e.logBytes)
		e.logCapped = true
	}
}

// readEvents drains one bridge subprocess's stderr until it exits. own is
// the *exec.Cmd this goroutine belongs to: the exit cleanup only clears the
// engine's state if that bridge is still the current one, so a stale
// goroutine from an already-replaced bridge (e.g. after SetToken kills it
// for a re-auth) can't clobber the new, healthy bridge's state — that race
// intermittently marked the fresh bridge "not running" (MUS-17).
func (e *Engine) readEvents(r io.Reader, own *exec.Cmd) {
	scanner := bufio.NewScanner(r)
	// Bump the buffer size — librespot can occasionally emit very long log
	// lines (e.g. stack traces) that exceed the default 64KB cap.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		var ev bridgeEvent
		isJSON := json.Unmarshal([]byte(line), &ev) == nil
		// Mirror to the debug log so we can diagnose later even if the bridge
		// crashes or emits non-JSON output — but skip the high-frequency
		// "spectrum" frames (tens per second, each a 128-float array). They
		// carry no diagnostic value and were the main driver of multi-GB logs.
		if !(isJSON && ev.Event == "spectrum") {
			e.writeLog(line)
		}
		if !isJSON {
			continue // non-JSON lines are librespot logs — captured above
		}

		posMs := int64(0)
		if ev.PositionMs != nil {
			posMs = *ev.PositionMs
			e.positionMs.Store(posMs)
		}

		switch ev.Event {
		case "playing":
			e.playing.Store(true)
		case "paused", "stopped", "end_of_track":
			e.playing.Store(false)
		case "spectrum":
			// Write available FFT data to SharedSpectrum
			if e.Spectrum != nil {
				var data SpectrumData
				if len(ev.Magnitudes) > 0 {
					for i, m := range ev.Magnitudes {
						if i < NumBins {
							data.Magnitudes[i] = m
						}
					}
				}
				if ev.Bass != nil {
					data.Bands.Bass = *ev.Bass
				}
				if ev.Mids != nil {
					data.Bands.Mids = *ev.Mids
				}
				if ev.Highs != nil {
					data.Bands.Highs = *ev.Highs
				}
				if ev.Energy != nil {
					data.Energy = *ev.Energy
				}
				if ev.Beat != nil {
					data.Beat = *ev.Beat
				}
				if ev.BeatIntensity != nil {
					data.BeatIntensity = *ev.BeatIntensity
				}
				if ev.Bpm != nil {
					data.BPM = *ev.Bpm
				}
				e.Spectrum.Write(data)
			}
			continue // don't emit spectrum as a TUI event (too frequent)
		}

		e.emit(Event{
			Kind:       ev.Event,
			PositionMs: posMs,
			Error:      ev.Message,
			TrackID:    ev.TrackID,
		})
	}

	// Bridge exited. Always reap our own process, but only clear the
	// engine's state (and tell the UI playback stopped) if we are still the
	// CURRENT bridge — if a new one has already been started, its state is
	// not ours to touch.
	_ = own.Wait()

	e.mu.Lock()
	current := e.cmd == own
	if current {
		if e.stdin != nil {
			e.stdin.Close()
			e.stdin = nil
		}
		e.cmd = nil
		e.started = false
		e.playing.Store(false)
	}
	e.mu.Unlock()

	if current {
		e.emit(Event{Kind: "stopped"})
	}
}

func (e *Engine) sendCmd(cmd bridgeCommand) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stdin == nil {
		return fmt.Errorf("bridge not running")
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(e.stdin, "%s\n", data)
	if err != nil {
		// Bridge stdin is broken — the process died under us. Kill/clear so
		// the next ensureBridge() spawns a fresh one (its readEvents
		// goroutine reaps it; the identity guard keeps a replacement safe).
		if e.cmd != nil && e.cmd.Process != nil {
			_ = e.cmd.Process.Kill()
		}
		e.stdin = nil
		e.started = false
	}
	return err
}

// PlayTrack starts playing a track by Spotify base62 ID.
// Lazily starts the bridge subprocess on first call.
//
// If the bridge died since the last command (librespot crash, or killed for
// a token refresh) the first write fails on the dead pipe — restart the
// bridge and retry once instead of surfacing a raw "bridge not running" to
// the user mid-track-change (MUS-17).
func (e *Engine) PlayTrack(trackID string) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		e.mu.Lock()
		err = e.ensureBridge()
		e.mu.Unlock()
		if err != nil {
			return err
		}

		err = e.sendCmd(bridgeCommand{
			Cmd:     "play",
			Token:   e.token,
			TrackID: trackID,
		})
		if err == nil {
			return nil
		}
	}
	return err
}

func (e *Engine) Pause() error {
	return e.sendCmd(bridgeCommand{Cmd: "pause"})
}

func (e *Engine) Resume() error {
	return e.sendCmd(bridgeCommand{Cmd: "resume"})
}

func (e *Engine) Stop() error {
	return e.sendCmd(bridgeCommand{Cmd: "stop"})
}

func (e *Engine) Seek(positionMs uint32) error {
	return e.sendCmd(bridgeCommand{Cmd: "seek", PositionMs: positionMs})
}

func (e *Engine) SetVolume(vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	e.volume.Store(int32(vol))
	return e.sendCmd(bridgeCommand{Cmd: "volume", Value: uint8(vol)})
}

func (e *Engine) Volume() int {
	return int(e.volume.Load())
}

// SetToken updates the OAuth access token used by the bridge for streaming
// authentication. Called on AuthSuccessMsg so a re-login picks up fresh
// scopes without the TUI needing to tear down and recreate the engine.
// Also kills the current bridge subprocess so the next play command
// spawns a new one that performs a fresh session.connect() with the new
// token — librespot does not support re-authentication on an existing
// session.
func (e *Engine) SetToken(token string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.token = token
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	if e.stdin != nil {
		_ = e.stdin.Close()
		e.stdin = nil
	}
	// Clear the state NOW rather than waiting for the readEvents goroutine
	// to observe the kill: a PlayTrack landing in that window used to see
	// started=true with a dead pipe and fail with "bridge not running"
	// (MUS-17). The old goroutine still reaps its own process, and the
	// identity guard keeps it from touching the replacement bridge's state.
	e.cmd = nil
	e.started = false
	e.playing.Store(false)
}

func (e *Engine) PositionMs() int64 {
	return e.positionMs.Load()
}

func (e *Engine) IsPlaying() bool {
	return e.playing.Load()
}

func (e *Engine) Started() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started
}

func (e *Engine) Events() <-chan Event {
	return e.events
}

func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stdin != nil {
		e.stdin.Close()
		e.stdin = nil
	}
	if e.cmd != nil && e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
		// Do NOT Wait here: the bridge's readEvents goroutine is the sole
		// reaper (exec.Cmd.Wait must only ever have one caller — a second
		// concurrent Wait is a data race).
		e.cmd = nil
	}
	if e.logFile != nil {
		_ = e.logFile.Close()
		e.logFile = nil
	}
	e.started = false
}

func (e *Engine) emit(ev Event) {
	select {
	case e.events <- ev:
	default:
	}
}
