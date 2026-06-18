package audio

import "sync"

const NumBins = 128
const WaveformSize = 512

type BandEnergy struct {
	Bass  float32
	Mids  float32
	Highs float32
}

type SpectrumData struct {
	Magnitudes  [NumBins]float32
	Peaks       [NumBins]float32
	Waveform    [WaveformSize]float32
	Bands       BandEnergy
	Energy      float32
	LeftEnergy  float32
	RightEnergy float32
	Beat        bool
	// BeatIntensity is a continuous beat envelope (0..1): spikes to 1.0 on an
	// onset and decays, for smooth beat-reactive rendering.
	BeatIntensity float32
	// BPM is the estimated tempo (0 until enough onsets are seen).
	BPM float32
}

// SharedSpectrum provides thread-safe access to spectrum data.
type SharedSpectrum struct {
	mu   sync.RWMutex
	data SpectrumData
}

func NewSharedSpectrum() *SharedSpectrum {
	return &SharedSpectrum{}
}

func (s *SharedSpectrum) Read() SpectrumData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *SharedSpectrum) Write(data SpectrumData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data
}
