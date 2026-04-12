use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

/// ASCII fire effect driven by audio energy
pub struct Flames {
    heat_map: Vec<Vec<f32>>,
    width: usize,
    height: usize,
    rng_state: u64,
}

impl Flames {
    pub fn new() -> Self {
        Self {
            heat_map: Vec::new(),
            width: 0,
            height: 0,
            rng_state: 123456789,
        }
    }

    fn next_rand(&mut self) -> f64 {
        self.rng_state ^= self.rng_state << 13;
        self.rng_state ^= self.rng_state >> 7;
        self.rng_state ^= self.rng_state << 17;
        (self.rng_state as f64) / (u64::MAX as f64)
    }

    fn ensure_size(&mut self, w: usize, h: usize) {
        if self.width != w || self.height != h {
            self.width = w;
            self.height = h;
            self.heat_map = vec![vec![0.0; w]; h];
        }
    }
}

impl Visualizer for Flames {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Flames",
            description: "ASCII fire driven by audio",
            index: 6,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, _dt: f64) {
        // Default size if not yet initialized (will adapt in render)
        if self.width == 0 || self.height == 0 {
            self.ensure_size(80, 24);
        }

        // Seed bottom row with audio energy
        let bottom = self.height - 1;
        for x in 0..self.width {
            let freq_pos = x as f32 / self.width as f32;
            let mag = spectrum.magnitude_at(freq_pos);
            let base = mag * 1.5 + spectrum.energy * 0.5;
            self.heat_map[bottom][x] = (base + self.next_rand() as f32 * 0.3).min(1.0);
        }

        // Propagate upward with cooling
        for y in 0..self.height - 1 {
            for x in 0..self.width {
                let below = y + 1;
                let left = if x > 0 { x - 1 } else { x };
                let right = if x + 1 < self.width { x + 1 } else { x };

                let avg = (self.heat_map[below][left]
                    + self.heat_map[below][x]
                    + self.heat_map[below][right]
                    + self.heat_map[below][x])
                    / 4.0;

                let cooling = 0.05 + self.next_rand() as f32 * 0.03;
                self.heat_map[y][x] = (avg - cooling).max(0.0);
            }
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 {
            return;
        }

        let fire_chars = [' ', '.', ':', '*', 's', 'S', '#', '%', '@'];

        for y in 0..area.height.min(self.height as u16) {
            for x in 0..area.width.min(self.width as u16) {
                let heat = self.heat_map[y as usize][x as usize];
                let char_idx =
                    (heat * (fire_chars.len() - 1) as f32).round() as usize;
                let ch = fire_chars[char_idx.min(fire_chars.len() - 1)];

                let color = if heat > 0.8 {
                    Color::Rgb(220, 230, 255) // bright white-blue
                } else if heat > 0.6 {
                    Color::Rgb(170, 130, 250) // bright purple
                } else if heat > 0.4 {
                    Color::Rgb(100, 100, 255) // blue
                } else if heat > 0.2 {
                    Color::Rgb(40, 150, 220) // cyan-blue
                } else if heat > 0.05 {
                    Color::Rgb(10, 80, 120) // dark teal
                } else {
                    continue;
                };

                buf[(area.x + x, area.y + y)]
                    .set_char(ch)
                    .set_style(Style::default().fg(color));
            }
        }
    }

    fn reset(&mut self) {
        self.heat_map.clear();
        self.width = 0;
        self.height = 0;
    }
}
