package audio

import (
	"encoding/binary"
	"io"
	"math"
	"math/cmplx"

	"github.com/madelynnblue/go-dsp/fft"
)

const (
	sampleRate  = 44100
	fftSize     = 2048
	channels    = 2
	bytesPerSmp = 2 // s16le
	frameBytes  = channels * bytesPerSmp
)

// Analyzer reads raw PCM (s16le, 44100Hz, stereo) from a reader,
// runs FFT, and writes SpectrumData to a SharedSpectrum.
type Analyzer struct {
	spectrum *SharedSpectrum
	buffer   []float64 // ring buffer of mono samples
	writePos int
	window   []float64 // Hann window
	peaks    [NumBins]float32
}

func NewAnalyzer(spectrum *SharedSpectrum) *Analyzer {
	// Precompute Hann window
	win := make([]float64, fftSize)
	for i := range win {
		win[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(fftSize-1)))
	}
	return &Analyzer{
		spectrum: spectrum,
		buffer:   make([]float64, fftSize),
		window:   win,
	}
}

// Run reads PCM from r (blocking) and continuously updates spectrum data.
// It also copies all data to w (the audio output) so sound plays.
// Call in a goroutine.
func (a *Analyzer) Run(r io.Reader, w io.Writer) {
	buf := make([]byte, frameBytes*512) // read ~512 frames at a time
	analysisCounter := 0

	for {
		n, err := r.Read(buf)
		if n > 0 {
			// Forward PCM to audio output
			if w != nil {
				w.Write(buf[:n])
			}

			// Decode s16le stereo to mono float64
			for i := 0; i+frameBytes <= n; i += frameBytes {
				left := int16(binary.LittleEndian.Uint16(buf[i:]))
				right := int16(binary.LittleEndian.Uint16(buf[i+2:]))
				mono := (float64(left) + float64(right)) / 2.0 / 32768.0

				a.buffer[a.writePos] = mono
				a.writePos = (a.writePos + 1) % fftSize
			}

			// Run FFT every ~4 reads (~170 Hz analysis rate)
			analysisCounter++
			if analysisCounter >= 4 {
				analysisCounter = 0
				a.analyze()
			}
		}
		if err != nil {
			break
		}
	}
}

func (a *Analyzer) analyze() {
	// Build windowed sample buffer (unwrap ring buffer)
	samples := make([]float64, fftSize)
	for i := 0; i < fftSize; i++ {
		idx := (a.writePos + i) % fftSize
		samples[i] = a.buffer[idx] * a.window[i]
	}

	// Run FFT
	result := fft.FFTReal(samples)

	// Convert to magnitudes and map to log-frequency bins
	var data SpectrumData
	halfFFT := len(result) / 2

	// Map FFT bins to NumBins log-frequency bins
	for i := 0; i < NumBins; i++ {
		// Log-frequency mapping: bin i covers frequencies from f1 to f2
		f1 := 20.0 * math.Pow(float64(sampleRate)/2.0/20.0, float64(i)/float64(NumBins))
		f2 := 20.0 * math.Pow(float64(sampleRate)/2.0/20.0, float64(i+1)/float64(NumBins))

		bin1 := int(f1 * float64(fftSize) / float64(sampleRate))
		bin2 := int(f2 * float64(fftSize) / float64(sampleRate))
		if bin1 < 1 {
			bin1 = 1
		}
		if bin2 >= halfFFT {
			bin2 = halfFFT - 1
		}
		if bin2 < bin1 {
			bin2 = bin1
		}

		// Average magnitude in this frequency range
		var sum float64
		count := 0
		for b := bin1; b <= bin2 && b < halfFFT; b++ {
			mag := cmplx.Abs(result[b])
			sum += mag
			count++
		}
		if count > 0 {
			sum /= float64(count)
		}

		// Convert to dB-like scale (log scale, normalized)
		db := 20.0 * math.Log10(sum+1e-10)
		normalized := (db + 80.0) / 80.0 // map ~-80dB..0dB to 0..1
		if normalized < 0 {
			normalized = 0
		}
		if normalized > 1 {
			normalized = 1
		}

		data.Magnitudes[i] = float32(normalized)

		// Peak hold
		if data.Magnitudes[i] > a.peaks[i] {
			a.peaks[i] = data.Magnitudes[i]
		} else {
			a.peaks[i] *= 0.97
		}
		data.Peaks[i] = a.peaks[i]
	}

	// Band energies
	bassEnd := NumBins * 15 / 100
	midsEnd := NumBins * 60 / 100
	var bassSum, midsSum, highsSum float32
	for i := 0; i < NumBins; i++ {
		if i < bassEnd {
			bassSum += data.Magnitudes[i]
		} else if i < midsEnd {
			midsSum += data.Magnitudes[i]
		} else {
			highsSum += data.Magnitudes[i]
		}
	}
	data.Bands.Bass = bassSum / float32(bassEnd)
	data.Bands.Mids = midsSum / float32(midsEnd-bassEnd)
	data.Bands.Highs = highsSum / float32(NumBins-midsEnd)
	data.Energy = (data.Bands.Bass + data.Bands.Mids + data.Bands.Highs) / 3.0

	// Simple beat detection: bass energy spike
	data.Beat = data.Bands.Bass > 0.6

	a.spectrum.Write(data)
}
