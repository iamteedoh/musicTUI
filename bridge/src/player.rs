use std::path::PathBuf;
use std::sync::atomic::AtomicU32;
use std::sync::{Arc, RwLock};
use std::time::Duration;

use librespot::core::cache::Cache;
use librespot::core::config::SessionConfig;
use librespot::core::session::Session;
use librespot::core::{SpotifyId, SpotifyUri};
use librespot::playback::audio_backend;
use librespot::playback::config::{AudioFormat, Bitrate, PlayerConfig};
use librespot::playback::mixer::{self, Mixer, MixerConfig};
use librespot::playback::player::{Player, PlayerEvent};
use ringbuf::traits::Split;
use ringbuf::HeapRb;
use thiserror::Error;
use tokio::sync::mpsc;
use tracing::{debug, info};

use crate::fft;
use crate::sink::AnalyzerSink;
use crate::spectrum::SpectrumData;

#[derive(Debug, Error)]
pub enum AudioError {
    #[error("Failed to connect session: {0}")]
    SessionConnect(String),

    #[error("Invalid track ID: {0}")]
    InvalidTrackId(String),

    #[error("No audio backend found")]
    NoBackend,

    #[error("No mixer found")]
    NoMixer,

    #[error("{0}")]
    Other(String),
}

/// Events emitted by the audio player, mapped for the TUI
#[derive(Debug, Clone)]
pub enum AudioEvent {
    Loading,
    Playing { position_ms: u32 },
    Paused { position_ms: u32 },
    PositionChanged { position_ms: u32 },
    Stopped,
    EndOfTrack,
    Unavailable { track_id: String },
}

/// Manages librespot Session and Player for audio playback
pub struct AudioPlayer {
    player: Arc<Player>,
    _session: Session,
    mixer: Arc<dyn Mixer>,
}

impl AudioPlayer {
    /// Create a new AudioPlayer connected to Spotify.
    /// Returns the player, a channel of audio events, and shared spectrum data for visualization.
    pub async fn new(
        access_token: &str,
        cache_dir: Option<PathBuf>,
    ) -> Result<(Self, mpsc::UnboundedReceiver<AudioEvent>, Arc<RwLock<SpectrumData>>), AudioError> {
        let session_config = SessionConfig::default();

        let cache = cache_dir
            .and_then(|dir| Cache::new(Some(dir), None, None, None).ok());

        // Connect session using OAuth access token
        let session = Session::new(session_config, cache);
        let credentials =
            librespot::core::authentication::Credentials::with_access_token(access_token);

        session
            .connect(credentials, false)
            .await
            .map_err(|e| AudioError::SessionConnect(e.to_string()))?;

        info!("librespot session connected");

        // Create software mixer for volume control
        let mixer_factory = mixer::find(Some("softvol"))
            .ok_or(AudioError::NoMixer)?;
        let mixer = mixer_factory(MixerConfig::default())
            .map_err(|e| AudioError::Other(format!("Failed to create mixer: {}", e)))?;
        let volume_getter = mixer.get_soft_volume();

        // Player config: max quality + periodic position updates
        let mut player_config = PlayerConfig::default();
        player_config.bitrate = Bitrate::Bitrate320;
        player_config.position_update_interval = Some(Duration::from_millis(500));

        // Create audio backend (rodio - default)
        let backend_fn = audio_backend::find(None)
            .ok_or(AudioError::NoBackend)?;

        // Set up visualization pipeline: ring buffer + FFT thread
        let ring = HeapRb::<f32>::new(8192);
        let (producer, consumer) = ring.split();
        let spectrum = Arc::new(RwLock::new(SpectrumData::default()));
        let left_energy = Arc::new(AtomicU32::new(0));
        let right_energy = Arc::new(AtomicU32::new(0));
        let _fft_handle = fft::spawn_fft_thread(
            consumer,
            spectrum.clone(),
            left_energy.clone(),
            right_energy.clone(),
        );

        // Wrap the rodio backend with our analyzer sink
        let le = left_energy;
        let re = right_energy;
        let player = Player::new(
            player_config,
            session.clone(),
            volume_getter,
            move || {
                let inner = backend_fn(None, AudioFormat::default());
                Box::new(AnalyzerSink::new(inner, producer, le, re))
            },
        );

        // Get event channel from player
        let mut event_channel = player.get_player_event_channel();

        // Forward relevant player events through our channel
        let (tx, rx) = mpsc::unbounded_channel();
        tokio::spawn(async move {
            while let Some(event) = event_channel.recv().await {
                if let Some(audio_event) = map_player_event(&event) {
                    if tx.send(audio_event).is_err() {
                        break;
                    }
                }
            }
        });

        Ok((
            Self {
                player,
                _session: session,
                mixer,
            },
            rx,
            spectrum,
        ))
    }

    /// Load and optionally start playing a track
    pub fn load_track(
        &self,
        track_id: &str,
        play: bool,
        position_ms: u32,
    ) -> Result<(), AudioError> {
        let id = SpotifyId::from_base62(track_id)
            .map_err(|e| AudioError::InvalidTrackId(format!("{}: {}", track_id, e)))?;
        debug!("Loading track: {} (play={}, pos={}ms)", track_id, play, position_ms);
        self.player.load(SpotifyUri::Track { id }, play, position_ms);
        Ok(())
    }

    pub fn pause(&self) {
        self.player.pause();
    }

    pub fn play(&self) {
        self.player.play();
    }

    pub fn stop(&self) {
        self.player.stop();
    }

    pub fn seek(&self, position_ms: u32) {
        self.player.seek(position_ms);
    }

    /// Set volume (0-65535 range, librespot native)
    pub fn set_volume(&self, volume: u16) {
        self.mixer.set_volume(volume);
    }

    /// Convert app volume (0-100) to librespot volume (0-65535) and set
    pub fn set_volume_percent(&self, percent: u8) {
        let volume = (percent.min(100) as u16) * 655;
        self.set_volume(volume);
    }

    /// Get current volume (0-65535)
    pub fn volume(&self) -> u16 {
        self.mixer.volume()
    }
}

fn map_player_event(event: &PlayerEvent) -> Option<AudioEvent> {
    match event {
        PlayerEvent::Loading { .. } => Some(AudioEvent::Loading),
        PlayerEvent::Playing { position_ms, .. } => Some(AudioEvent::Playing {
            position_ms: *position_ms,
        }),
        PlayerEvent::Paused { position_ms, .. } => Some(AudioEvent::Paused {
            position_ms: *position_ms,
        }),
        PlayerEvent::PositionChanged { position_ms, .. } => Some(AudioEvent::PositionChanged {
            position_ms: *position_ms,
        }),
        PlayerEvent::Stopped { .. } => Some(AudioEvent::Stopped),
        PlayerEvent::EndOfTrack { .. } => Some(AudioEvent::EndOfTrack),
        PlayerEvent::Unavailable { track_id, .. } => Some(AudioEvent::Unavailable {
            track_id: format!("{}", track_id),
        }),
        _ => None,
    }
}
