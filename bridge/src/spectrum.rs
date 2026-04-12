/// Frequency band energy levels
#[derive(Debug, Clone, Default)]
pub struct BandEnergy {
    pub bass: f32,
    pub mids: f32,
    pub highs: f32,
}

/// Processed audio spectrum data shared between FFT thread and TUI renderer.
///
/// Updated by the FFT processor at ~60 Hz and read by visualizers during render.
#[derive(Debug, Clone)]
pub struct SpectrumData {
    /// Frequency magnitude bins (logarithmically scaled), typically 64-128 bins
    pub magnitudes: Vec<f32>,

    /// Raw waveform samples for the current window (for oscilloscope/waveform views)
    pub waveform: Vec<f32>,

    /// Peak values per bin (with slow decay for "peak hold" effect)
    pub peaks: Vec<f32>,

    /// Pre-computed band energy
    pub bands: BandEnergy,

    /// Overall energy (RMS of magnitudes)
    pub energy: f32,

    /// Real stereo channel energy (RMS from raw audio, not FFT)
    pub left_energy: f32,
    pub right_energy: f32,

    /// Beat detection flag
    pub beat: bool,

    /// Sample rate of the audio stream
    pub sample_rate: u32,
}

impl Default for SpectrumData {
    fn default() -> Self {
        let num_bins = 128;
        Self {
            magnitudes: vec![0.0; num_bins],
            waveform: vec![0.0; 1024],
            peaks: vec![0.0; num_bins],
            bands: BandEnergy::default(),
            energy: 0.0,
            left_energy: 0.0,
            right_energy: 0.0,
            beat: false,
            sample_rate: 44100,
        }
    }
}

impl SpectrumData {
    /// Get magnitude for a normalized position (0.0 = lowest freq, 1.0 = highest)
    pub fn magnitude_at(&self, pos: f32) -> f32 {
        if self.magnitudes.is_empty() {
            return 0.0;
        }
        let idx = (pos * (self.magnitudes.len() - 1) as f32).round() as usize;
        self.magnitudes[idx.min(self.magnitudes.len() - 1)]
    }
}
