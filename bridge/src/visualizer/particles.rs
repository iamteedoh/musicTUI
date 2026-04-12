use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

struct Particle {
    x: f64,
    y: f64,
    vx: f64,
    vy: f64,
    life: f64,
    max_life: f64,
    char: char,
    freq_band: usize, // which frequency band spawned this particle
}

const NUM_FREQ_BANDS: usize = 8;

/// Band colors: turquoise → blue → purple
const BAND_COLORS: [(u8, u8, u8); NUM_FREQ_BANDS] = [
    (0, 210, 180),   // sub bass - turquoise
    (20, 195, 210),  // bass - turquoise-cyan
    (40, 175, 235),  // low mids - cyan-blue
    (60, 145, 250),  // mids - light blue
    (80, 115, 255),  // upper mids - blue
    (115, 95, 250),  // presence - blue-indigo
    (150, 75, 240),  // brilliance - indigo
    (185, 60, 225),  // air - purple
];

/// Particle system driven by audio frequency spectrum
pub struct Particles {
    particles: Vec<Particle>,
    rng_state: u64,
    band_levels: [f32; NUM_FREQ_BANDS],
    band_spawn_acc: [f64; NUM_FREQ_BANDS],
}

impl Particles {
    pub fn new() -> Self {
        Self {
            particles: Vec::new(),
            rng_state: 42,
            band_levels: [0.0; NUM_FREQ_BANDS],
            band_spawn_acc: [0.0; NUM_FREQ_BANDS],
        }
    }

    fn next_rand(&mut self) -> f64 {
        // Simple xorshift64
        self.rng_state ^= self.rng_state << 13;
        self.rng_state ^= self.rng_state >> 7;
        self.rng_state ^= self.rng_state << 17;
        (self.rng_state as f64) / (u64::MAX as f64)
    }
}

impl Visualizer for Particles {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Particles",
            description: "Audio-reactive particle system",
            index: 8,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        // Compute per-band energy from full spectrum
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

        // Spawn particles per frequency band
        let chars = ['·', '•', '*', '✦', '✧', '◦'];
        for band in 0..NUM_FREQ_BANDS {
            let level = self.band_levels[band] as f64;
            let spawn_rate = level * 30.0 + 1.0;
            self.band_spawn_acc[band] += spawn_rate * dt;

            while self.band_spawn_acc[band] >= 1.0 {
                self.band_spawn_acc[band] -= 1.0;

                // Direction based on band position: low freq = downward, high freq = upward
                let band_t = band as f64 / NUM_FREQ_BANDS as f64;
                let base_angle = (1.0 - band_t) * std::f64::consts::PI * 0.5 // low = right
                    + band_t * std::f64::consts::PI * 1.5; // high = up
                let spread = self.next_rand() * std::f64::consts::PI * 0.6 - std::f64::consts::PI * 0.3;
                let angle = base_angle + spread;
                let speed = 5.0 + self.next_rand() * 15.0 * level;
                let ch = chars[(self.next_rand() * chars.len() as f64) as usize % chars.len()];
                let life = 1.0 + self.next_rand() * 2.0;

                self.particles.push(Particle {
                    x: 0.5,
                    y: 0.5,
                    vx: angle.cos() * speed,
                    vy: angle.sin() * speed,
                    life,
                    max_life: life,
                    char: ch,
                    freq_band: band,
                });
            }
        }

        // Update particles
        for p in &mut self.particles {
            p.x += p.vx * dt * 0.02;
            p.y += p.vy * dt * 0.02;
            p.vy += dt * 2.0; // gravity
            p.life -= dt;
        }

        self.particles.retain(|p| p.life > 0.0);

        // Cap particle count
        if self.particles.len() > 600 {
            self.particles.drain(0..self.particles.len() - 600);
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width == 0 || area.height == 0 {
            return;
        }

        for p in &self.particles {
            let x = (p.x * area.width as f64).round() as i32;
            let y = (p.y * area.height as f64).round() as i32;

            if x >= 0
                && x < area.width as i32
                && y >= 0
                && y < area.height as i32
            {
                let alpha = (p.life / p.max_life).max(0.0) as f32;
                let (br, bg, bb) = BAND_COLORS[p.freq_band.min(NUM_FREQ_BANDS - 1)];
                let color = Color::Rgb(
                    (br as f32 * alpha) as u8,
                    (bg as f32 * alpha) as u8,
                    (bb as f32 * alpha) as u8,
                );

                buf[(area.x + x as u16, area.y + y as u16)]
                    .set_char(p.char)
                    .set_style(Style::default().fg(color));
            }
        }
    }

    fn reset(&mut self) {
        self.particles.clear();
        self.band_levels = [0.0; NUM_FREQ_BANDS];
        self.band_spawn_acc = [0.0; NUM_FREQ_BANDS];
    }
}
