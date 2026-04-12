use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::Color;
use ratatui::widgets::canvas::{Canvas, Line};
use ratatui::widgets::Widget;

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// Radial frequency display arranged in a circle with high-res Canvas
pub struct Circular {
    magnitudes: Vec<f32>,
    rotation: f64,
}

impl Circular {
    pub fn new() -> Self {
        Self {
            magnitudes: Vec::new(),
            rotation: 0.0,
        }
    }
}

impl Visualizer for Circular {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Circular",
            description: "High-resolution Braille radial display",
            index: 5,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        let num = spectrum.magnitudes.len();
        self.magnitudes.resize(num, 0.0);

        // Asymmetric EMA: fast attack (~35ms), snappy release (~155ms to 90%)
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        for i in 0..num {
            let target = spectrum.magnitudes[i];
            let rate = if target > self.magnitudes[i] { attack } else { release };
            self.magnitudes[i] += (target - self.magnitudes[i]) * rate;
        }

        self.rotation += dt * 0.5;
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 || self.magnitudes.is_empty() {
            return;
        }

        let canvas = Canvas::default()
            .x_bounds([-1.0, 1.0])
            .y_bounds([-1.0, 1.0])
            .paint(|ctx| {
                let num_points = self.magnitudes.len();
                let base_radius = 0.3;
                let max_radius = 0.9;

                for (i, &mag) in self.magnitudes.iter().enumerate() {
                    let angle = (i as f64 / num_points as f64) * std::f64::consts::TAU + self.rotation;
                    let radius = base_radius + mag as f64 * (max_radius - base_radius);
                    
                    let x2 = angle.cos() * radius;
                    let y2 = angle.sin() * radius;
                    let x1 = angle.cos() * base_radius;
                    let y1 = angle.sin() * base_radius;

                    let t = i as f32 / num_points as f32;
                    let color = gradient_color(t);

                    ctx.draw(&Line {
                        x1,
                        y1,
                        x2,
                        y2,
                        color,
                    });
                }
            });

        canvas.render(area, buf);
    }

    fn reset(&mut self) {
        self.magnitudes.clear();
        self.rotation = 0.0;
    }
}

/// Turquoise → blue → purple gradient
fn gradient_color(t: f32) -> Color {
    let t = t.clamp(0.0, 1.0);
    let (r, g, b) = if t < 0.5 {
        let s = t * 2.0;
        (s * 80.0, 210.0 - s * 95.0, 180.0 + s * 75.0)
    } else {
        let s = (t - 0.5) * 2.0;
        (80.0 + s * 105.0, 115.0 - s * 55.0, 255.0 - s * 30.0)
    };
    Color::Rgb(r as u8, g as u8, b as u8)
}
