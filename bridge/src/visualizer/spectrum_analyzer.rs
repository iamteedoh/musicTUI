use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::Color;
use ratatui::widgets::canvas::{Canvas, Line};
use ratatui::widgets::Widget;

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// Horizontal spectrum with sub-character precision and smooth gradient
pub struct SpectrumAnalyzer {
    magnitudes: Vec<f32>,
}

impl SpectrumAnalyzer {
    pub fn new() -> Self {
        Self {
            magnitudes: Vec::new(),
        }
    }
}

fn lerp(a: f32, b: f32, t: f32) -> f32 {
    a + (b - a) * t
}

/// Per-row gradient: low freq (left) = teal, high freq (right) = violet
fn column_color(t: f32) -> Color {
    let t = t.clamp(0.0, 1.0);
    let (r, g, b) = if t < 0.5 {
        let s = t * 2.0;
        (lerp(0.0, 60.0, s), lerp(210.0, 140.0, s), lerp(180.0, 255.0, s))
    } else {
        let s = (t - 0.5) * 2.0;
        (lerp(60.0, 200.0, s), lerp(140.0, 60.0, s), 255.0)
    };
    Color::Rgb(r as u8, g as u8, b as u8)
}

impl Visualizer for SpectrumAnalyzer {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Spectrum Analyzer",
            description: "High-resolution Braille spectrum bars",
            index: 4,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        let num = spectrum.magnitudes.len();
        self.magnitudes.resize(num, 0.0);

        // Asymmetric EMA: fast attack (~35ms), snappy release (~190ms to 90%)
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        for i in 0..num {
            let target = spectrum.magnitudes[i];
            let rate = if target > self.magnitudes[i] { attack } else { release };
            self.magnitudes[i] += (target - self.magnitudes[i]) * rate;
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 || self.magnitudes.is_empty() {
            return;
        }

        let bins = self.magnitudes.len();
        
        let canvas = Canvas::default()
            .x_bounds([0.0, bins as f64])
            .y_bounds([0.0, 1.0])
            .paint(|ctx| {
                for i in 0..bins {
                    let mag = self.magnitudes[i];
                    if mag > 0.01 { // Only draw if visible
                        let color = column_color(i as f32 / bins as f32);
                        // Draw a vertical bar using Line for sub-pixel precision
                        ctx.draw(&Line {
                            x1: i as f64,
                            y1: 0.0,
                            x2: i as f64,
                            y2: mag as f64,
                            color,
                        });
                        // Draw a thicker segment so bars look fuller on high-res displays
                        ctx.draw(&Line {
                            x1: i as f64 + 0.5,
                            y1: 0.0,
                            x2: i as f64 + 0.5,
                            y2: mag as f64,
                            color,
                        });
                    }
                }
            });

        canvas.render(area, buf);
    }

    fn reset(&mut self) {
        self.magnitudes.clear();
    }
}
