use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

struct RainDrop {
    x: u16,
    y: f64,
    speed: f64,
    length: u16,
    chars: Vec<char>,
    freq_band: usize, // which frequency band drives this drop (0..NUM_FREQ_BANDS)
}

const NUM_FREQ_BANDS: usize = 16;

/// Matrix-style digital rain modulated by audio
pub struct DigitalRain {
    drops: Vec<RainDrop>,
    rng_state: u64,
    energy: f32,
    band_levels: [f32; NUM_FREQ_BANDS],
}


impl DigitalRain {
    pub fn new() -> Self {
        Self {
            drops: Vec::new(),
            rng_state: 987654321,
            energy: 0.0,
            band_levels: [0.0; NUM_FREQ_BANDS],
        }
    }

    fn next_rand(&mut self) -> f64 {
        self.rng_state ^= self.rng_state << 13;
        self.rng_state ^= self.rng_state >> 7;
        self.rng_state ^= self.rng_state << 17;
        (self.rng_state as f64) / (u64::MAX as f64)
    }

    fn random_char(&mut self) -> char {
        let katakana_start = 0x30A0u32;
        let offset = (self.next_rand() * 96.0) as u32;
        char::from_u32(katakana_start + offset).unwrap_or('ア')
    }
}

impl Visualizer for DigitalRain {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Digital Rain",
            description: "Matrix-style rain modulated by audio",
            index: 7,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        self.energy = spectrum.energy;

        // Compute per-band energy from full spectrum (16 bands across 128 bins)
        let mags = &spectrum.magnitudes;
        let num_bins = mags.len();
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        for band in 0..NUM_FREQ_BANDS {
            let start = band * num_bins / NUM_FREQ_BANDS;
            let end = ((band + 1) * num_bins / NUM_FREQ_BANDS).min(num_bins);
            let target = if end > start {
                (mags[start..end].iter().map(|v| v * v).sum::<f32>() / (end - start) as f32).sqrt()
            } else {
                0.0
            };
            let rate = if target > self.band_levels[band] { attack } else { release };
            self.band_levels[band] += (target - self.band_levels[band]) * rate;
        }

        // Spawn drops per frequency band — columns map to bands
        // Use 200 as max column range; render clips to actual area
        let w = 200.0f64;
        for band in 0..NUM_FREQ_BANDS {
            let level = self.band_levels[band] as f64;
            let spawn_rate = 1.0 + level * 25.0;
            if self.next_rand() < spawn_rate * dt {
                let band_start = (band as f64 / NUM_FREQ_BANDS as f64 * w) as u16;
                let band_end = (((band + 1) as f64 / NUM_FREQ_BANDS as f64) * w) as u16;
                let x = band_start + (self.next_rand() * (band_end.saturating_sub(band_start)) as f64) as u16;
                let length = 4 + (self.next_rand() * 12.0 + level * 8.0) as u16;
                let mut chars = Vec::with_capacity(length as usize);
                for _ in 0..length {
                    chars.push(self.random_char());
                }
                let speed = 5.0 + self.next_rand() * 10.0 + level * 10.0;
                self.drops.push(RainDrop {
                    x,
                    y: -(length as f64),
                    speed,
                    length,
                    chars,
                    freq_band: band,
                });
            }
        }

        // Update drop positions
        let num_drops = self.drops.len();
        let mut mutations: Vec<(bool, f64, char)> = Vec::with_capacity(num_drops);
        for _ in 0..num_drops {
            let should_mutate = self.next_rand() < 0.1;
            let idx_rand = self.next_rand();
            let new_char = self.random_char();
            mutations.push((should_mutate, idx_rand, new_char));
        }

        for (drop, (should_mutate, idx_rand, new_char)) in
            self.drops.iter_mut().zip(mutations.into_iter())
        {
            let band_energy = self.band_levels[drop.freq_band.min(NUM_FREQ_BANDS - 1)] as f64;
            drop.speed *= 1.0 + band_energy * 0.01;
            drop.y += drop.speed * dt;

            if should_mutate {
                let idx = (idx_rand * drop.chars.len() as f64) as usize;
                if idx < drop.chars.len() {
                    drop.chars[idx] = new_char;
                }
            }
        }

        // Remove off-screen drops
        self.drops.retain(|d| d.y < 200.0);

        if self.drops.len() > 500 {
            self.drops.drain(0..self.drops.len() - 500);
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 {
            return;
        }

        // Band color gradient: turquoise → blue → purple
        let band_color = |band: usize, fade: f32| -> Color {
            let t = band as f32 / NUM_FREQ_BANDS as f32;
            let (r, g, b) = if t < 0.5 {
                let s = t * 2.0;
                (fade * s * 80.0, fade * (210.0 - s * 95.0), fade * (180.0 + s * 75.0))
            } else {
                let s = (t - 0.5) * 2.0;
                (fade * (80.0 + s * 105.0), fade * (115.0 - s * 55.0), fade * (255.0 - s * 30.0))
            };
            Color::Rgb(r as u8, g as u8, b as u8)
        };

        for drop in &self.drops {
            if drop.x >= area.width {
                continue;
            }

            for (i, &ch) in drop.chars.iter().enumerate() {
                let y = drop.y as i32 + i as i32;
                if y < 0 || y >= area.height as i32 {
                    continue;
                }

                let is_head = i == drop.chars.len() - 1;
                let fade = 1.0 - (i as f32 / drop.length as f32);

                let color = if is_head {
                    Color::Rgb(180, 240, 255)
                } else {
                    band_color(drop.freq_band, fade)
                };

                buf[(area.x + drop.x, area.y + y as u16)]
                    .set_char(ch)
                    .set_style(Style::default().fg(color));
            }
        }
    }

    fn reset(&mut self) {
        self.drops.clear();
    }
}
