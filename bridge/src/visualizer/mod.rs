use ratatui::buffer::Buffer;
use ratatui::layout::Rect;

use crate::spectrum::SpectrumData;

pub mod circular;
pub mod digital_rain;
pub mod flames;
pub mod frequency_bars;
pub mod galaxy;
pub mod oscilloscope;
pub mod particles;
pub mod peak_meter;
pub mod spectrum_analyzer;
pub mod waveform;

/// Metadata about a visualizer
#[derive(Debug, Clone)]
pub struct VisualizerInfo {
    pub name: &'static str,
    pub description: &'static str,
    pub index: usize,
}

/// Trait for audio visualizer implementations.
///
/// Each visualizer maintains its own state and renders into a ratatui Buffer.
/// The `update` method is called with new spectrum data at the render frame rate,
/// and `render` draws the current state into the terminal buffer.
pub trait Visualizer: Send {
    /// Return metadata about this visualizer
    fn info(&self) -> VisualizerInfo;

    /// Update internal state with new spectrum data
    fn update(&mut self, spectrum: &SpectrumData, dt: f64);

    /// Render current state into the terminal buffer
    fn render(&self, area: Rect, buf: &mut Buffer);

    /// Reset all internal state (e.g. when switching visualizers)
    fn reset(&mut self);
}

/// Create all available visualizers in order
pub fn create_all() -> Vec<Box<dyn Visualizer>> {
    vec![
        Box::new(peak_meter::PeakMeter::new()),
        Box::new(frequency_bars::FrequencyBars::new()),
        Box::new(waveform::Waveform::new()),
        Box::new(oscilloscope::Oscilloscope::new()),
        Box::new(spectrum_analyzer::SpectrumAnalyzer::new()),
        Box::new(circular::Circular::new()),
        Box::new(flames::Flames::new()),
        Box::new(digital_rain::DigitalRain::new()),
        Box::new(particles::Particles::new()),
        Box::new(galaxy::Galaxy::new()),
    ]
}
