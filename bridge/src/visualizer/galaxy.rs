use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

const NUM_ARMS: usize = 8;

struct Star {
    angle: f64,
    radius: f64,
    arm: usize,
    brightness: f32,
    freq_idx: usize, // index into smoothed magnitudes
}

/// Spiral galaxy visualization driven by full audio spectrum
pub struct Galaxy {
    stars: Vec<Star>,
    rotation: f64,
    energy: f32,
    magnitudes: Vec<f32>,
    rng_state: u64,
}

impl Galaxy {
    pub fn new() -> Self {
        let mut g = Self {
            stars: Vec::new(),
            rotation: 0.0,
            energy: 0.0,
            magnitudes: Vec::new(),
            rng_state: 314159265,
        };
        g.generate_stars();
        g
    }

    fn next_rand(&mut self) -> f64 {
        self.rng_state ^= self.rng_state << 13;
        self.rng_state ^= self.rng_state >> 7;
        self.rng_state ^= self.rng_state << 17;
        (self.rng_state as f64) / (u64::MAX as f64)
    }

    fn generate_stars(&mut self) {
        let stars_per_arm = 120;

        for arm in 0..NUM_ARMS {
            for i in 0..stars_per_arm {
                let t = i as f64 / stars_per_arm as f64;
                let radius = 0.05 + t * 0.9;
                let arm_offset = (arm as f64 / NUM_ARMS as f64) * std::f64::consts::TAU;
                let spiral = t * std::f64::consts::TAU * 2.0;
                let scatter = self.next_rand() * 0.2 - 0.1;
                let radius_jitter = self.next_rand() * 0.06;
                let brightness = 0.5 + self.next_rand() as f32 * 0.5;
                // Map each star to a frequency bin based on its position along the arm
                let freq_idx = (t * 127.0) as usize;

                self.stars.push(Star {
                    angle: arm_offset + spiral + scatter,
                    radius: radius + radius_jitter,
                    arm,
                    brightness,
                    freq_idx,
                });
            }
        }
    }
}

impl Visualizer for Galaxy {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Galaxy",
            description: "Spiral galaxy driven by audio",
            index: 9,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        self.energy = spectrum.energy;
        // Faster base rotation so direction is clearly visible
        let speed = 0.5 + spectrum.energy as f64 * 3.0;
        self.rotation += speed * dt;

        // Smooth full spectrum magnitudes with asymmetric EMA
        let num = spectrum.magnitudes.len();
        self.magnitudes.resize(num, 0.0);
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        for i in 0..num {
            let target = spectrum.magnitudes[i];
            let rate = if target > self.magnitudes[i] { attack } else { release };
            self.magnitudes[i] += (target - self.magnitudes[i]) * rate;
        }

        // Drive star brightness from individual frequency bins
        // Higher floor so stars are always visible, big boost when active
        let num_bins = self.magnitudes.len().max(1);
        for star in &mut self.stars {
            let idx = star.freq_idx.min(num_bins - 1);
            let mag = self.magnitudes[idx];
            // Boost curve: square root makes low values brighter
            let boosted = mag.sqrt();
            star.brightness = 0.35 + boosted * 0.65;
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width < 10 || area.height < 5 {
            return;
        }

        let cx = area.width as f64 / 2.0;
        let cy = area.height as f64 / 2.0;
        let scale_x = cx * 0.9;
        let scale_y = cy * 0.9;

        let star_chars = ['·', '•', '●', '★', '✦'];
        let arm_colors: [(u8, u8, u8); NUM_ARMS] = [
            (0, 220, 190),   // turquoise
            (20, 205, 215),  // turquoise-cyan
            (45, 180, 240),  // cyan-blue
            (65, 150, 255),  // light blue
            (85, 120, 255),  // blue
            (120, 100, 250), // blue-indigo
            (155, 80, 240),  // indigo
            (190, 65, 230),  // purple
        ];

        for star in &self.stars {
            let angle = star.angle + self.rotation;
            // Stronger pulse on active stars
            let pulse = 1.0 + self.energy as f64 * 0.35 * (star.radius * 8.0).sin();

            let x = cx + angle.cos() * star.radius * scale_x * 2.0 * pulse;
            let y = cy + angle.sin() * star.radius * scale_y * pulse;

            let px = x.round() as i32;
            let py = y.round() as i32;

            if px >= 0
                && px < area.width as i32
                && py >= 0
                && py < area.height as i32
            {
                let char_idx =
                    (star.brightness * (star_chars.len() - 1) as f32).round() as usize;
                let ch = star_chars[char_idx.min(star_chars.len() - 1)];

                let (br, bg, bb) = arm_colors[star.arm % arm_colors.len()];
                // Brightness floor so even dim stars show the arm color
                let f = (star.brightness * 0.7 + 0.3).min(1.0);
                let color = Color::Rgb(
                    (br as f32 * f) as u8,
                    (bg as f32 * f) as u8,
                    (bb as f32 * f) as u8,
                );

                buf[(area.x + px as u16, area.y + py as u16)]
                    .set_char(ch)
                    .set_style(Style::default().fg(color));
            }
        }

        // Galaxy center glow — bigger and brighter
        let center_brightness = (0.6 + self.energy * 0.4).min(1.0);
        let cb = (center_brightness * 255.0) as u8;
        let center_x = cx as u16;
        let center_y = cy as u16;
        buf[(area.x + center_x, area.y + center_y)]
            .set_char('◉')
            .set_style(Style::default().fg(Color::Rgb(cb, cb, (cb as f32 * 0.8) as u8)));
        // Soft glow ring around center
        let glow = (cb as f32 * 0.4) as u8;
        for &(dx, dy) in &[(1i16, 0i16), (-1, 0), (0, 1), (0, -1)] {
            let gx = center_x as i16 + dx;
            let gy = center_y as i16 + dy;
            if gx >= 0 && (gx as u16) < area.width && gy >= 0 && (gy as u16) < area.height {
                buf[(area.x + gx as u16, area.y + gy as u16)]
                    .set_char('·')
                    .set_style(Style::default().fg(Color::Rgb(glow, glow, glow)));
            }
        }
    }

    fn reset(&mut self) {
        self.rotation = 0.0;
        self.energy = 0.0;
        self.magnitudes.clear();
        self.stars.clear();
        self.generate_stars();
    }
}
