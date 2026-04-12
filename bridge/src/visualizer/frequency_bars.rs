use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// Continuous frequency mountain with eighth-block vertical resolution
pub struct FrequencyBars {
    bar_heights: Vec<f32>,
}

impl FrequencyBars {
    pub fn new() -> Self {
        Self {
            bar_heights: Vec::new(),
        }
    }
}

fn lerp(a: f32, b: f32, t: f32) -> f32 {
    a + (b - a) * t
}

/// Smooth RGB gradient: teal → blue → violet
fn gradient(t: f32) -> Color {
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

impl Visualizer for FrequencyBars {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Frequency Bars",
            description: "Smooth frequency mountain",
            index: 1,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        let num = spectrum.magnitudes.len();
        self.bar_heights.resize(num, 0.0);

        // Asymmetric EMA: fast attack (~35ms), snappy release (~190ms to 90%)
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        for i in 0..num {
            let target = spectrum.magnitudes[i];
            let rate = if target > self.bar_heights[i] { attack } else { release };
            self.bar_heights[i] += (target - self.bar_heights[i]) * rate;
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 || self.bar_heights.is_empty() {
            return;
        }

        let cols = area.width as usize;
        let bins = self.bar_heights.len();
        if bins == 0 {
            return;
        }
        let eighth_h = area.height as usize * 8;
        const EIGHTHS: [&str; 9] = [" ", "▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"];

        for col in 0..cols {
            let x = area.x + col as u16;

            // Interpolate between bins for smooth horizontal contour
            let t = col as f32 / cols as f32 * (bins - 1) as f32;
            let lo = (t.floor() as usize).min(bins - 1);
            let hi = (lo + 1).min(bins - 1);
            let frac = t - lo as f32;
            let mag = self.bar_heights[lo] * (1.0 - frac) + self.bar_heights[hi] * frac;

            let h = (mag * eighth_h as f32).round().clamp(0.0, eighth_h as f32) as usize;
            let full = h / 8;
            let rem = h % 8;

            // Full block cells from bottom up
            for row in 0..full {
                let y = area.y + area.height - 1 - row as u16;
                if y < area.y {
                    break;
                }
                let ht = row as f32 / area.height as f32;
                buf[(x, y)]
                    .set_symbol("█")
                    .set_style(Style::default().fg(gradient(ht)));
            }

            // Fractional top cell with eighth-block precision
            if rem > 0 && full < area.height as usize {
                let y = area.y + area.height - 1 - full as u16;
                if y >= area.y {
                    let ht = full as f32 / area.height as f32;
                    buf[(x, y)]
                        .set_symbol(EIGHTHS[rem])
                        .set_style(Style::default().fg(gradient(ht)));
                }
            }
        }
    }

    fn reset(&mut self) {
        self.bar_heights.clear();
    }
}
