use ratatui::buffer::Buffer;
use ratatui::layout::Rect;
use ratatui::style::{Color, Style};

use crate::spectrum::SpectrumData;
use super::{Visualizer, VisualizerInfo};

const NUM_BANDS: usize = 8;

/// Band definitions: label, start bin (inclusive), end bin (exclusive)
/// Based on 128-bin quadratic mapping at 44.1kHz / 2048 FFT
const BAND_DEFS: [(&str, usize, usize); NUM_BANDS] = [
    ("SUB  ", 0, 5),      // Sub Bass    ~0-60 Hz
    ("BASS ", 5, 14),     // Bass        ~60-250 Hz
    ("LMID ", 14, 22),    // Low Mids    ~250-600 Hz
    ("MID  ", 22, 40),    // Mids        ~600-2kHz
    ("UMID ", 40, 56),    // Upper Mids  ~2-4kHz
    ("PRES ", 56, 72),    // Presence    ~4-7kHz
    ("BRIL ", 72, 96),    // Brilliance  ~7-14kHz
    ("AIR  ", 96, 128),   // Air         ~14-22kHz
];

/// 8-band color gradient: turquoise → blue → purple
const BAND_COLORS: [Color; NUM_BANDS] = [
    Color::Rgb(0, 210, 180),   // sub bass - turquoise
    Color::Rgb(20, 195, 210),  // bass - turquoise-cyan
    Color::Rgb(40, 175, 235),  // low mids - cyan-blue
    Color::Rgb(60, 145, 250),  // mids - light blue
    Color::Rgb(80, 115, 255),  // upper mids - blue
    Color::Rgb(115, 95, 250),  // presence - blue-indigo
    Color::Rgb(150, 75, 240),  // brilliance - indigo
    Color::Rgb(185, 60, 225),  // air - purple
];

/// Classic VU / peak meter with 8-band frequency display
pub struct PeakMeter {
    left_level: f32,
    right_level: f32,
    bands: [f32; NUM_BANDS],
}

impl PeakMeter {
    pub fn new() -> Self {
        Self {
            left_level: 0.0,
            right_level: 0.0,
            bands: [0.0; NUM_BANDS],
        }
    }
}

/// Smooth turquoise → blue → purple gradient for VU meters
fn meter_color(t: f32) -> Color {
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

impl Visualizer for PeakMeter {
    fn info(&self) -> VisualizerInfo {
        VisualizerInfo {
            name: "Peak Meter",
            description: "VU meters with 8-band EQ",
            index: 0,
        }
    }

    fn update(&mut self, spectrum: &SpectrumData, dt: f64) {
        // Asymmetric EMA: fast attack (~35ms), snappy release (~155ms to 90%)
        let attack = (dt * 55.0).min(1.0) as f32;
        let release = (dt * 15.0).min(1.0) as f32;

        let targets = [
            (spectrum.left_energy, &mut self.left_level),
            (spectrum.right_energy, &mut self.right_level),
        ];
        for (target, level) in targets {
            let rate = if target > *level { attack } else { release };
            *level += (target - *level) * rate;
        }

        // Compute 8-band energy directly from magnitudes
        let mags = &spectrum.magnitudes;
        let num_bins = mags.len();
        for (i, &(_, start, end)) in BAND_DEFS.iter().enumerate() {
            let s = start.min(num_bins);
            let e = end.min(num_bins);
            let target = if e > s {
                (mags[s..e].iter().map(|v| v * v).sum::<f32>() / (e - s) as f32).sqrt()
            } else {
                0.0
            };
            let rate = if target > self.bands[i] { attack } else { release };
            self.bands[i] += (target - self.bands[i]) * rate;
        }
    }

    fn render(&self, area: Rect, buf: &mut Buffer) {
        if area.width < 10 || area.height < 5 {
            return;
        }

        let meter_width = area.width - 8;
        let eighths: [char; 8] = [' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'];

        // L/R stereo meters (with a blank row between them)
        let labels = [("L ", self.left_level), ("R ", self.right_level)];
        let lr_rows: [u16; 2] = [0, 2]; // row 0 = L, row 1 = gap, row 2 = R
        for (i, (label, level)) in labels.iter().enumerate() {
            let y = area.y + lr_rows[i];
            if y >= area.y + area.height {
                break;
            }

            for (j, ch) in label.chars().enumerate() {
                if j < 2 {
                    buf[(area.x + j as u16, y)]
                        .set_char(ch)
                        .set_style(Style::default().fg(Color::White));
                }
            }

            let precise = *level * meter_width as f32;
            let full = precise.floor() as u16;
            let frac = ((precise - full as f32) * 8.0).round() as usize;

            for x in 0..meter_width {
                let pos = area.x + 3 + x;
                let ratio = x as f32 / meter_width as f32;

                if x < full {
                    buf[(pos, y)]
                        .set_symbol("█")
                        .set_style(Style::default().fg(meter_color(ratio)));
                } else if x == full && frac > 0 {
                    buf[(pos, y)]
                        .set_char(eighths[frac.min(7)])
                        .set_style(Style::default().fg(meter_color(ratio)));
                } else {
                    buf[(pos, y)]
                        .set_symbol("░")
                        .set_style(Style::default().fg(Color::Rgb(40, 40, 50)));
                }
            }
        }

        // 8-band frequency meters (skip a row after L/R)
        let band_y_start = area.y + 4;
        let label_width = 5u16;

        for (i, (&(label, _, _), &level)) in
            BAND_DEFS.iter().zip(self.bands.iter()).enumerate()
        {
            let y = band_y_start + i as u16;
            if y >= area.y + area.height {
                break;
            }

            let color = BAND_COLORS[i];

            // Label
            for (j, ch) in label.chars().enumerate() {
                if (j as u16) < label_width && (area.x + j as u16) < area.x + area.width {
                    buf[(area.x + j as u16, y)]
                        .set_char(ch)
                        .set_style(Style::default().fg(color));
                }
            }

            // Meter bar
            let bar_width = area.width - label_width - 1;
            let precise = level * bar_width as f32;
            let full = precise.floor() as u16;
            let frac = ((precise - full as f32) * 8.0).round() as usize;

            for x in 0..bar_width {
                let pos = area.x + label_width + 1 + x;
                if pos >= area.x + area.width {
                    break;
                }
                if x < full {
                    buf[(pos, y)]
                        .set_symbol("█")
                        .set_style(Style::default().fg(color));
                } else if x == full && frac > 0 {
                    buf[(pos, y)]
                        .set_char(eighths[frac.min(7)])
                        .set_style(Style::default().fg(color));
                }
            }
        }
    }

    fn reset(&mut self) {
        self.left_level = 0.0;
        self.right_level = 0.0;
        self.bands = [0.0; NUM_BANDS];
    }
}
