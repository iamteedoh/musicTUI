package audio

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
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
	Event      string    `json:"event"`
	TrackID    string    `json:"track_id,omitempty"`
	PositionMs *int64    `json:"position_ms,omitempty"`
	Message    string    `json:"message,omitempty"`
	Magnitudes []float32 `json:"magnitudes,omitempty"`
	Bass       *float32  `json:"bass,omitempty"`
	Mids       *float32  `json:"mids,omitempty"`
	Highs      *float32  `json:"highs,omitempty"`
	Energy     *float32  `json:"energy,omitempty"`
	Beat       *bool     `json:"beat,omitempty"`
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

	// Spectrum analysis
	Spectrum *SharedSpectrum
	analyzer *Analyzer
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
		analyzer:   NewAnalyzer(spectrum),
	}
	e.volume.Store(75)
	return e
}

func (e *Engine) ensureBridge() error {
	if e.started {
		return nil
	}

	e.cmd = exec.Command(e.bridgePath)

	// Fully isolate the subprocess from the terminal:
	// - Create a new process group so it can't access our controlling terminal
	// - Pipe stdin/stderr for our JSON protocol
	// - Redirect stdout to /dev/null (librespot logs)
	e.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // new process group — cannot steal terminal
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
	go e.readEvents(stderr)

	return nil
}

func (e *Engine) readEvents(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		var ev bridgeEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // skip non-JSON lines (librespot logs)
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
	e.emit(Event{Kind: "stopped"})
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
	return err
}

// PlayTrack starts playing a track by Spotify base62 ID.
// Lazily starts the bridge subprocess on first call.
func (e *Engine) PlayTrack(trackID string) error {
	e.mu.Lock()
	err := e.ensureBridge()
	e.mu.Unlock()
	if err != nil {
		return err
	}

	return e.sendCmd(bridgeCommand{
		Cmd:     "play",
		Token:   e.token,
		TrackID: trackID,
	})
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
		_ = e.cmd.Wait()
		e.cmd = nil
	}
	e.started = false
}

func (e *Engine) emit(ev Event) {
	select {
	case e.events <- ev:
	default:
	}
}
