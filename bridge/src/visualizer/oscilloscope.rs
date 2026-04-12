use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::Color;
use ratatui::widgets::canvas::{Canvas, Line};
use ratatui::widgets::Widget;

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// Classic oscilloscope display with high-resolution Braille continuous lines
pub struct Oscilloscope {
    samples: Vec<f32>,
    prev_samples: Vec<f32>,
}

impl Oscilloscope {
    pub fn new() -> Self {
        Self {
            samples: Vec::new(),
            prev_samples: Vec::new(),
        }
    }
}

impl Visualizer for Oscilloscope {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Oscilloscope",
            description: "High-resolution Braille oscilloscope",
            index: 3,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, _dt: f64) {
        self.prev_samples = std::mem::take(&mut self.samples);
        self.samples = spectrum.waveform.clone();
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 || self.samples.is_empty() {
            return;
        }

        let canvas = Canvas::default()
            .x_bounds([0.0, self.samples.len() as f64])
            .y_bounds([-1.0, 1.0])
            .paint(|ctx| {
                // Draw center line
                ctx.draw(&Line {
                    x1: 0.0,
                    y1: 0.0,
                    x2: self.samples.len() as f64,
                    y2: 0.0,
                    color: Color::Rgb(25, 30, 50),
                });

                // Faded previous trace (phosphor glow)
                if !self.prev_samples.is_empty() {
                    let len_prev = self.prev_samples.len();
                    for i in 0..len_prev.saturating_sub(1) {
                        let sample_y1 = self.prev_samples[i] as f64;
                        let sample_y2 = self.prev_samples[i + 1] as f64;
                        ctx.draw(&Line {
                            x1: i as f64,
                            y1: sample_y1,
                            x2: (i + 1) as f64,
                            y2: sample_y2,
                            color: Color::Rgb(30, 30, 80),
                        });
                    }
                }

                // Current trace using smooth gradient segments
                let len = self.samples.len();
                for i in 0..len.saturating_sub(1) {
                    let sample_y1 = self.samples[i] as f64;
                    let sample_y2 = self.samples[i + 1] as f64;
                    
                    let t = i as f32 / len as f32;
                    let (r, g, b) = if t < 0.5 {
                        let s = t * 2.0;
                        (s * 80.0, 210.0 - s * 95.0, 180.0 + s * 75.0)
                    } else {
                        let s = (t - 0.5) * 2.0;
                        (80.0 + s * 105.0, 115.0 - s * 55.0, 255.0 - s * 30.0)
                    };
                    
                    ctx.draw(&Line {
                        x1: i as f64,
                        y1: sample_y1,
                        x2: (i + 1) as f64,
                        y2: sample_y2,
                        color: Color::Rgb(r as u8, g as u8, b as u8),
                    });
                }
            });

        // Use the ratatui Widget trait to render the canvas directly to the slice of the buffer
        canvas.render(area, buf);
    }

    fn reset(&mut self) {
        self.samples.clear();
        self.prev_samples.clear();
    }
}
