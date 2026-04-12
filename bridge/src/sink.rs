use std::sync::atomic::{AtomicU32, AtomicUsize, Ordering};
use std::sync::Arc;

use librespot::playback::audio_backend::{Sink, SinkResult};
use librespot::playback::convert::Converter;
use librespot::playback::decoder::AudioPacket;
use ringbuf::traits::Producer;
use ringbuf::HeapProd;
use tracing::{debug, info};

pub struct AnalyzerSink {
    inner: Box<dyn Sink>,
    producer: HeapProd<f32>,
    left_energy: Arc<AtomicU32>,
    right_energy: Arc<AtomicU32>,
    write_count: Arc<AtomicUsize>,
}

impl AnalyzerSink {
    pub fn new(
        inner: Box<dyn Sink>,
        producer: HeapProd<f32>,
        left_energy: Arc<AtomicU32>,
        right_energy: Arc<AtomicU32>,
    ) -> Self {
        info!("AnalyzerSink created");
        Self {
            inner,
            producer,
            left_energy,
            right_energy,
            write_count: Arc::new(AtomicUsize::new(0)),
        }
    }
}

impl Sink for AnalyzerSink {
    fn start(&mut self) -> SinkResult<()> {
        self.inner.start()
    }

    fn stop(&mut self) -> SinkResult<()> {
        self.inner.stop()
    }

    fn write(&mut self, packet: AudioPacket, converter: &mut Converter) -> SinkResult<()> {
        let count = self.write_count.fetch_add(1, Ordering::Relaxed);
        if count == 0 {
            info!("AnalyzerSink::write first call, packet variant: {}", match &packet {
                AudioPacket::Samples(s) => format!("Samples(len={})", s.len()),
                _ => "Raw".to_string(),
            });
        } else if count % 1000 == 0 {
            debug!("AnalyzerSink::write call #{}", count);
        }
        if let AudioPacket::Samples(ref samples) = packet {
            // Compute real L/R RMS energy from stereo samples
            let frame_count = samples.len() / 2;
            if frame_count > 0 {
                let mut left_sum = 0.0f64;
                let mut right_sum = 0.0f64;
                for chunk in samples.chunks_exact(2) {
                    left_sum += chunk[0] * chunk[0];
                    right_sum += chunk[1] * chunk[1];
                }
                let left_rms = (left_sum / frame_count as f64).sqrt() as f32;
                let right_rms = (right_sum / frame_count as f64).sqrt() as f32;
                self.left_energy.store(left_rms.to_bits(), Ordering::Relaxed);
                self.right_energy.store(right_rms.to_bits(), Ordering::Relaxed);
            }

            // Downmix interleaved f64 stereo to mono f32 for FFT
            let mono: Vec<f32> = samples
                .chunks_exact(2)
                .map(|lr| ((lr[0] + lr[1]) * 0.5) as f32)
                .collect();

            // Push to ring buffer; overflow is silently dropped (best-effort)
            self.producer.push_slice(&mono);
        }

        // Always forward the original packet to the real audio backend
        self.inner.write(packet, converter)
    }
}
