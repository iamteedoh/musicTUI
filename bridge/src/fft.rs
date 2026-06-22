use std::collections::VecDeque;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::{Arc, RwLock};
use std::thread::{self, JoinHandle};
use std::time::Duration;

use ringbuf::traits::Consumer;
use ringbuf::HeapCons;
use rustfft::num_complex::Complex;
use rustfft::FftPlanner;

use crate::spectrum::{BandEnergy, SpectrumData};

const FFT_SIZE: usize = 2048;
const HOP_SIZE: usize = 256;
const NUM_BINS: usize = 128;
const WAVEFORM_SIZE: usize = 512;
const PEAK_DECAY: f32 = 0.99;
const BIN_SMOOTH: f32 = 0.65; // EMA alpha — fast response, let visualizers add their own smoothing
const LR_SMOOTH: f32 = 0.45;

// ── Beat / onset detection (spectral flux on low frequencies) ────────────
// The FFT thread runs at the hop rate: 44100 / HOP_SIZE ≈ 172 frames/sec.
const FRAMES_PER_SEC: f32 = 44100.0 / HOP_SIZE as f32;
// Bins (of NUM_BINS log-spaced bins) to watch for onsets. Narrowed to the
// kick band (~0–200 Hz) — the steady low-end pulse that defines the tempo —
// so snare/synth/vocal transients don't trigger false beats.
const FLUX_BASS_BINS: usize = 12;
// Rolling window for the adaptive threshold (~500 ms of flux history).
const FLUX_HISTORY_LEN: usize = 86;
// Threshold = mean + K * stddev of recent flux. Higher K = fewer, surer beats.
const FLUX_THRESHOLD_K: f32 = 1.4;
// Absolute floor so silence/near-silence never triggers.
const FLUX_MIN: f32 = 0.01;
// Beat envelope decay per frame (~0.88 → pulse fades over ~80 ms).
const BEAT_DECAY: f32 = 0.88;
// Refractory period: ignore new onsets for ~120 ms after one fires, so a single
// hit can't double-trigger (caps detected tempo around 500 BPM).
const REFRACTORY_FRAMES: usize = (0.12 * FRAMES_PER_SEC) as usize;
// Phase-locked beat clock: how strongly each detected onset nudges the steady
// clock toward it (0 = ignore onsets, 1 = snap). Small = smooth, stable lock.
const PHASE_CORRECTION: f32 = 0.10;
// Only onsets landing within this much of the expected beat (in phase units)
// steer the clock — off-beat onsets (eighth-note bass, syncopation) are ignored.
const PHASE_GATE: f32 = 0.22;

// ── Tempo estimation (autocorrelation of the flux envelope) ──────────────
// Inter-onset intervals are too noisy for tempo; autocorrelating a few seconds
// of the onset envelope robustly finds the dominant beat period instead.
const TEMPO_MIN_BPM: f32 = 85.0;
const TEMPO_MAX_BPM: f32 = 175.0;
const TEMPO_ENV_LEN: usize = 520; // ~3s of flux history at ~172 Hz
const TEMPO_UPDATE_FRAMES: usize = 43; // recompute tempo ~4x/sec
// Lag (in frames) bounds for the BPM search range.
const TEMPO_MIN_LAG: usize = (60.0 * FRAMES_PER_SEC / TEMPO_MAX_BPM) as usize;
const TEMPO_MAX_LAG: usize = (60.0 * FRAMES_PER_SEC / TEMPO_MIN_BPM) as usize;

/// Default audio-output-buffer compensation, in milliseconds. The visualizer
/// analyses samples as librespot produces them, but they're only heard after
/// rodio's + ALSA's output buffer — so without compensation the viz runs ahead
/// of the speakers. Tuned by ear with the CAVA-style (low-smoothing, crisp)
/// visualizer, which exposes the lead the old blurry viz used to mask.
/// Override at runtime with MUSICTUI_VIZ_DELAY_MS (0 disables).
const VIZ_DELAY_MS_DEFAULT: f32 = 260.0;

pub fn spawn_fft_thread(
    mut consumer: HeapCons<f32>,
    spectrum: Arc<RwLock<SpectrumData>>,
    left_energy: Arc<AtomicU32>,
    right_energy: Arc<AtomicU32>,
) -> JoinHandle<()> {
    // Resolve the output-latency compensation (samples of mono audio to hold
    // back before analysis). Tunable without a rebuild via MUSICTUI_VIZ_DELAY_MS.
    let viz_delay_samples: usize = {
        let ms = std::env::var("MUSICTUI_VIZ_DELAY_MS")
            .ok()
            .and_then(|s| s.trim().parse::<f32>().ok())
            .unwrap_or(VIZ_DELAY_MS_DEFAULT)
            .max(0.0);
        eprintln!("[FFT] viz delay = {:.0}ms", ms);
        (ms * 44.1) as usize // 44100 Hz, mono
    };

    thread::spawn(move || {
        let mut planner = FftPlanner::<f32>::new();
        let fft = planner.plan_fft_forward(FFT_SIZE);

        let window: Vec<f32> = (0..FFT_SIZE)
            .map(|i| {
                let t = std::f32::consts::PI * 2.0 * i as f32 / (FFT_SIZE - 1) as f32;
                0.5 * (1.0 - t.cos())
            })
            .collect();

        let mut circ_buf = vec![0.0f32; FFT_SIZE];
        let mut circ_pos: usize = 0;
        let mut samples_since_fft: usize = 0;

        let mut peaks = vec![0.0f32; NUM_BINS];

        // Beat/onset detection state (spectral flux on the low-frequency bins).
        let mut prev_bass = vec![0.0f32; FLUX_BASS_BINS];
        let mut flux_history: VecDeque<f32> = VecDeque::with_capacity(FLUX_HISTORY_LEN);
        let mut beat_envelope: f32 = 0.0;
        let mut refractory: usize = 0;
        let mut frames_since_onset: u32 = 0;
        let mut prev_flux: f32 = 0.0;
        let mut beat_phase: f32 = 0.0; // phase-locked beat clock, 0..1
        let mut flux_env: VecDeque<f32> = VecDeque::with_capacity(TEMPO_ENV_LEN); // for autocorrelation
        let mut tempo_timer: usize = 0;
        let mut smooth_bpm: f32 = 0.0;

        // Persistent smoothed output — the key to eliminating jitter
        let mut smooth_bins = vec![0.0f32; NUM_BINS];
        let mut smooth_left: f32 = 0.0;
        let mut smooth_right: f32 = 0.0;

        let mut fft_input = vec![Complex::new(0.0f32, 0.0); FFT_SIZE];
        let mut read_buf = vec![0.0f32; HOP_SIZE];

        // Delay buffers: compensate for audio output latency so viz syncs with speakers
        let mut delay_buf: VecDeque<f32> =
            VecDeque::with_capacity(viz_delay_samples + HOP_SIZE);
        // Parallel L/R delay — captures atomic values at write time, reads them delayed
        let lr_delay_hops = viz_delay_samples / HOP_SIZE + 1;
        let mut lr_delay: VecDeque<(f32, f32)> =
            VecDeque::with_capacity(lr_delay_hops + 4);

        loop {
            let count = consumer.pop_slice(&mut read_buf);
            if count == 0 {
                thread::sleep(Duration::from_millis(2));
                continue;
            }

            // Push new samples into delay buffer
            for &s in &read_buf[..count] {
                delay_buf.push_back(s);
            }

            // Capture current L/R energy for delayed use (one reading per consumer batch)
            let lr_now = (
                f32::from_bits(left_energy.load(Ordering::Relaxed)),
                f32::from_bits(right_energy.load(Ordering::Relaxed)),
            );

            // Only process samples that have been delayed long enough
            while delay_buf.len() > viz_delay_samples {
                let sample = delay_buf.pop_front().unwrap();
                circ_buf[circ_pos] = sample;
                circ_pos = (circ_pos + 1) % FFT_SIZE;
                samples_since_fft += 1;

                if samples_since_fft < HOP_SIZE {
                    continue;
                }
                samples_since_fft = 0;

                // Push current L/R reading into delay queue at hop rate
                lr_delay.push_back(lr_now);

                for i in 0..FFT_SIZE {
                    let idx = (circ_pos + i) % FFT_SIZE;
                    fft_input[i] = Complex::new(circ_buf[idx] * window[i], 0.0);
                }

                fft.process(&mut fft_input);

                let half = FFT_SIZE / 2;
                let raw_mags: Vec<f32> = fft_input[..half]
                    .iter()
                    .map(|c| c.norm() / half as f32)
                    .collect();

                // Logarithmic binning with MAX — preserves beat peaks
                let mut bins = vec![0.0f32; NUM_BINS];
                for bin_idx in 0..NUM_BINS {
                    let t0 = bin_idx as f32 / NUM_BINS as f32;
                    let t1 = (bin_idx + 1) as f32 / NUM_BINS as f32;
                    let start = (t0 * t0 * half as f32) as usize;
                    let end =
                        ((t1 * t1 * half as f32) as usize).max(start + 1).min(half);

                    let mut max_val = 0.0f32;
                    for i in start..end {
                        max_val = max_val.max(raw_mags[i]);
                    }
                    bins[bin_idx] = max_val;
                }

                // Frequency-dependent scaling (boost bass + mids more)
                for (i, val) in bins.iter_mut().enumerate() {
                    let t = i as f32 / NUM_BINS as f32;
                    let scale = 3.0 + t * 8.0;
                    *val *= scale;
                }

                // dB normalization with wider dynamic range (-60dB floor)
                for val in bins.iter_mut() {
                    *val = if *val > 0.0 {
                        ((20.0 * val.log10() + 60.0) / 60.0).clamp(0.0, 1.0)
                    } else {
                        0.0
                    };
                }

                // Spatial smoothing across frequency bins (3-tap kernel)
                let mut spatial = vec![0.0f32; NUM_BINS];
                for i in 0..NUM_BINS {
                    let left = if i > 0 { bins[i - 1] } else { bins[i] };
                    let right = if i < NUM_BINS - 1 { bins[i + 1] } else { bins[i] };
                    spatial[i] = left * 0.15 + bins[i] * 0.7 + right * 0.15;
                }

                // Symmetric EMA smoothing at 172 Hz — eliminates jitter, low latency
                for i in 0..NUM_BINS {
                    smooth_bins[i] += (spatial[i] - smooth_bins[i]) * BIN_SMOOTH;
                }

                // Peak hold (on smoothed data)
                for (i, &mag) in smooth_bins.iter().enumerate() {
                    peaks[i] = if mag > peaks[i] {
                        mag
                    } else {
                        peaks[i] * PEAK_DECAY
                    };
                }

                // Band energy from smoothed bins
                let bass =
                    (smooth_bins[..12].iter().map(|v| v * v).sum::<f32>() / 12.0).sqrt();
                let mids =
                    (smooth_bins[12..62].iter().map(|v| v * v).sum::<f32>() / 50.0).sqrt();
                let highs =
                    (smooth_bins[62..].iter().map(|v| v * v).sum::<f32>() / 66.0).sqrt();

                let energy = (smooth_bins.iter().map(|v| v * v).sum::<f32>()
                    / NUM_BINS as f32)
                    .sqrt();

                // ── Beat tracking ───────────────────────────────────────
                // 1) Onset detection. Spectral flux on the kick band (positive
                //    frame-to-frame increases), with an adaptive threshold and a
                //    rising-edge trigger so each kick fires exactly once. Onsets
                //    feed the tempo estimate and phase clock — NOT the visual
                //    pulse directly (that's what made it twitchy before).
                let mut flux = 0.0f32;
                for i in 0..FLUX_BASS_BINS {
                    let diff = bins[i] - prev_bass[i];
                    if diff > 0.0 {
                        flux += diff;
                    }
                    prev_bass[i] = bins[i];
                }

                // Adaptive threshold from recent flux: mean + K * stddev.
                let (mean, std) = if flux_history.is_empty() {
                    (0.0, 0.0)
                } else {
                    let n = flux_history.len() as f32;
                    let m = flux_history.iter().sum::<f32>() / n;
                    let var =
                        flux_history.iter().map(|v| (v - m) * (v - m)).sum::<f32>() / n;
                    (m, var.sqrt())
                };
                let threshold = mean + FLUX_THRESHOLD_K * std;

                // Rising edge: flux crosses up through the threshold, outside the
                // refractory window.
                let onset = refractory == 0
                    && flux > FLUX_MIN
                    && flux > threshold
                    && prev_flux <= threshold;

                if onset {
                    refractory = REFRACTORY_FRAMES;
                    frames_since_onset = 0;

                    // Phase-lock: nudge the clock so on-beat onsets settle on
                    // phase 0. The phase gate ignores onsets far from the
                    // expected beat (off-beat bass notes, syncopation), so the
                    // bassline's eighth notes can't drag the clock to half-beats.
                    if smooth_bpm > 0.0 {
                        let err = if beat_phase < 0.5 {
                            beat_phase
                        } else {
                            beat_phase - 1.0
                        };
                        if err.abs() < PHASE_GATE {
                            beat_phase -= err * PHASE_CORRECTION;
                            if beat_phase < 0.0 {
                                beat_phase += 1.0;
                            }
                        }
                    }
                } else {
                    frames_since_onset = frames_since_onset.saturating_add(1);
                }
                if refractory > 0 {
                    refractory -= 1;
                }
                prev_flux = flux;
                if flux_history.len() >= FLUX_HISTORY_LEN {
                    flux_history.pop_front();
                }
                flux_history.push_back(flux);

                // Tempo via autocorrelation of the flux envelope. Recomputed a
                // few times per second over a multi-second window — robust to
                // which subdivisions trigger, unlike inter-onset intervals.
                if flux_env.len() >= TEMPO_ENV_LEN {
                    flux_env.pop_front();
                }
                flux_env.push_back(flux);
                tempo_timer += 1;
                if tempo_timer >= TEMPO_UPDATE_FRAMES && flux_env.len() >= TEMPO_MAX_LAG * 2 {
                    tempo_timer = 0;
                    let env: Vec<f32> = flux_env.iter().copied().collect();
                    let n = env.len();
                    let hi = (TEMPO_MAX_LAG * 2).min(n - 1);
                    let mut ac = vec![0.0f32; hi + 1];
                    for lag in TEMPO_MIN_LAG..=hi {
                        let mut sum = 0.0f32;
                        for i in lag..n {
                            sum += env[i] * env[i - lag];
                        }
                        ac[lag] = sum / (n - lag) as f32;
                    }
                    // Pick the lag with the strongest autocorrelation, boosted by
                    // its octave harmonic so the true tempo beats half-tempo.
                    let mut best_lag = 0usize;
                    let mut best = 0.0f32;
                    for lag in TEMPO_MIN_LAG..=TEMPO_MAX_LAG {
                        let mut score = ac[lag];
                        if lag * 2 <= hi {
                            score += 0.5 * ac[lag * 2];
                        }
                        if score > best {
                            best = score;
                            best_lag = lag;
                        }
                    }
                    if best_lag > 0 && best > 0.0 {
                        let bpm = (60.0 * FRAMES_PER_SEC / best_lag as f32)
                            .clamp(TEMPO_MIN_BPM, TEMPO_MAX_BPM);
                        smooth_bpm = if smooth_bpm == 0.0 {
                            bpm
                        } else {
                            smooth_bpm + (bpm - smooth_bpm) * 0.10
                        };
                    }
                }

                // 2) Pulse generation. Once a tempo is locked, the beat envelope
                //    is driven by a steady phase clock running at that BPM — this
                //    is what makes the visual *match the tempo* rather than react
                //    to every transient. Before lock, pulse on raw onsets.
                let mut beat = false;
                if smooth_bpm > 0.0 {
                    beat_phase += smooth_bpm / 60.0 / FRAMES_PER_SEC;
                    if beat_phase >= 1.0 {
                        beat_phase -= 1.0;
                        beat_envelope = 1.0;
                        beat = true;
                    } else {
                        beat_envelope *= BEAT_DECAY;
                    }
                } else if onset {
                    beat_envelope = 1.0;
                    beat = true;
                } else {
                    beat_envelope *= BEAT_DECAY;
                }

                // Drop tempo + clock if onsets stop (track end, silence, ambient).
                if frames_since_onset as f32 > 2.0 * FRAMES_PER_SEC {
                    smooth_bpm = 0.0;
                    beat_phase = 0.0;
                    flux_env.clear();
                    tempo_timer = 0;
                }

                // Waveform
                let mut waveform = vec![0.0f32; WAVEFORM_SIZE];
                for i in 0..WAVEFORM_SIZE {
                    let idx = (circ_pos + FFT_SIZE - WAVEFORM_SIZE + i) % FFT_SIZE;
                    waveform[i] = circ_buf[idx];
                }

                // L/R energy: read from delayed queue (synced with audio output)
                let (delayed_left, delayed_right) = if lr_delay.len() > lr_delay_hops {
                    lr_delay.pop_front().unwrap()
                } else {
                    lr_now
                };
                smooth_left += (delayed_left - smooth_left) * LR_SMOOTH;
                smooth_right += (delayed_right - smooth_right) * LR_SMOOTH;

                let left_e = if smooth_left > 0.0 {
                    ((20.0 * smooth_left.log10() + 40.0) / 40.0).clamp(0.0, 1.0)
                } else {
                    0.0
                };
                let right_e = if smooth_right > 0.0 {
                    ((20.0 * smooth_right.log10() + 40.0) / 40.0).clamp(0.0, 1.0)
                } else {
                    0.0
                };

                // Write to shared spectrum
                if let Ok(mut spec) = spectrum.try_write() {
                    spec.magnitudes.copy_from_slice(&smooth_bins);
                    spec.peaks.copy_from_slice(&peaks);
                    spec.waveform = waveform;
                    spec.bands = BandEnergy { bass, mids, highs };
                    spec.energy = energy;
                    spec.left_energy = left_e;
                    spec.right_energy = right_e;
                    spec.beat = beat;
                    spec.beat_intensity = beat_envelope;
                    spec.bpm = smooth_bpm;
                }
            }
        }
    })
}
