use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// Real-time audio waveform with half-block precision
pub struct Waveform {
    samples: Vec<f32>,
}

impl Waveform {
    pub fn new() -> Self {
        Self {
            samples: Vec::new(),
        }
    }
}

/// Turquoise → blue → purple based on amplitude
fn wave_color(amplitude: f32) -> Color {
    let t = amplitude.clamp(0.0, 1.0);
    let (r, g, b) = if t < 0.5 {
        let s = t * 2.0;
        (s * 80.0, 210.0 - s * 95.0, 180.0 + s * 75.0)
    } else {
        let s = (t - 0.5) * 2.0;
        (80.0 + s * 105.0, 115.0 - s * 55.0, 255.0 - s * 30.0)
    };
    Color::Rgb(r as u8, g as u8, b as u8)
}

impl Visualizer for Waveform {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Waveform",
            description: "Oscilloscope with half-block precision",
            index: 2,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, _dt: f64) {
        self.samples = spectrum.waveform.clone();
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 || self.samples.is_empty() {
            return;
        }

        let h = area.height as f32;
        let n = self.samples.len();
        let half_total = (h * 2.0) as i32;

        // Subtle center line
        let mid = area.y + area.height / 2;
        for x in 0..area.width {
            buf[(area.x + x, mid)]
                .set_symbol("─")
                .set_style(Style::default().fg(Color::Rgb(35, 40, 50)));
        }

        let mut prev_y_half: Option<i32> = None;

        for x in 0..area.width {
            let t = x as f32 / area.width as f32;
            let si = (t * n as f32) as usize;
            let sample = self.samples[si.min(n - 1)];

            // Map sample (-1..1) to half-pixel y (0 = top, 2*h = bottom)
            let y_half = ((0.5 - sample * 0.5) * h * 2.0)
                .round()
                .clamp(0.0, (half_total - 1) as f32) as i32;

            let amplitude = sample.abs();
            let color = wave_color(amplitude);

            // Connect to previous column with vertical line
            let (y_min, y_max) = if let Some(prev) = prev_y_half {
                (y_half.min(prev), y_half.max(prev))
            } else {
                (y_half, y_half)
            };

            let cell_min = (y_min / 2).max(0) as u16;
            let cell_max = (y_max / 2).min(area.height as i32 - 1) as u16;

            for cell in cell_min..=cell_max {
                let y = area.y + cell;
                let top_half = cell as i32 * 2;
                let bot_half = top_half + 1;

                let top_in = top_half >= y_min && top_half <= y_max;
                let bot_in = bot_half >= y_min && bot_half <= y_max;

                let sym = match (top_in, bot_in) {
                    (true, true) => "█",
                    (true, false) => "▀",
                    (false, true) => "▄",
                    (false, false) => continue,
                };

                buf[(area.x + x, y)]
                    .set_symbol(sym)
                    .set_style(Style::default().fg(color));
            }

            prev_y_half = Some(y_half);
        }
    }

    fn reset(&mut self) {
        self.samples.clear();
    }
}
