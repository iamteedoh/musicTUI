//! Minimal audio player bridge for the Go TUI.
//! Reads JSON commands from stdin, plays audio via librespot, outputs events on stderr.
//!
//! Commands (one JSON per line on stdin):
//!   {"cmd":"play","token":"...","track_id":"..."}
//!   {"cmd":"pause"}
//!   {"cmd":"resume"}
//!   {"cmd":"stop"}
//!   {"cmd":"seek","position_ms":12345}
//!   {"cmd":"volume","value":75}
//!
//! Events (one JSON per line on stderr):
//!   {"event":"loading","track_id":"..."}
//!   {"event":"playing","position_ms":0}
//!   {"event":"paused","position_ms":12345}
//!   {"event":"position","position_ms":12345}
//!   {"event":"stopped"}
//!   {"event":"end_of_track"}
//!   {"event":"error","message":"..."}

use std::io::{self, BufRead};
use std::sync::{Arc, RwLock};

use audio_engine::{AudioPlayer, AudioEvent};
use audio_engine::spectrum::SpectrumData;
use serde::{Deserialize, Serialize};
use tokio::sync::mpsc;

#[derive(Deserialize)]
struct Command {
    cmd: String,
    #[serde(default)]
    token: String,
    #[serde(default)]
    track_id: String,
    #[serde(default)]
    position_ms: u32,
    #[serde(default)]
    value: u8,
}

#[derive(Serialize)]
struct EventOut {
    event: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    track_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    position_ms: Option<u32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    magnitudes: Option<Vec<f32>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    bass: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    mids: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    highs: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    energy: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    beat: Option<bool>,
}

fn emit(ev: EventOut) {
    if let Ok(json) = serde_json::to_string(&ev) {
        eprintln!("{}", json);
    }
}

#[tokio::main]
async fn main() {
    // Initialize env_logger so librespot's log:: macros actually emit to
    // stderr. Without this, every librespot error/warning was silently
    // swallowed, leaving us with an empty bridge.log and nothing to
    // diagnose from. Default to warn so we don't spam users; callers
    // can override with RUST_LOG.
    env_logger::Builder::from_env(env_logger::Env::default().default_filter_or("warn"))
        .target(env_logger::Target::Stderr)
        .init();

    let (cmd_tx, mut cmd_rx) = mpsc::unbounded_channel::<Command>();

    // Read commands from stdin in a blocking thread
    std::thread::spawn(move || {
        let stdin = io::stdin();
        for line in stdin.lock().lines() {
            match line {
                Ok(line) if !line.trim().is_empty() => {
                    if let Ok(cmd) = serde_json::from_str::<Command>(&line) {
                        if cmd_tx.send(cmd).is_err() {
                            break;
                        }
                    }
                }
                Err(_) => break,
                _ => {}
            }
        }
    });

    let mut player: Option<AudioPlayer> = None;
    let mut event_rx: Option<mpsc::UnboundedReceiver<AudioEvent>> = None;
    let mut spectrum: Option<Arc<RwLock<SpectrumData>>> = None;

    // Spectrum at 10Hz
    let mut spectrum_interval = tokio::time::interval(tokio::time::Duration::from_millis(100));

    loop {
        tokio::select! {
            _ = spectrum_interval.tick() => {
                if let Some(ref spec) = spectrum {
                    if let Ok(data) = spec.read() {
                        let mags: Vec<f32> = data.magnitudes.clone();
                        emit(EventOut {
                            event: "spectrum".into(),
                            magnitudes: Some(mags),
                            bass: Some(data.bands.bass),
                            mids: Some(data.bands.mids),
                            highs: Some(data.bands.highs),
                            energy: Some(data.energy),
                            beat: Some(data.beat),
                            ..Default::default()
                        });
                    }
                }
            }
            Some(cmd) = cmd_rx.recv() => {
                match cmd.cmd.as_str() {
                    "play" => {
                        // Create player if needed
                        if player.is_none() && !cmd.token.is_empty() {
                            let cache_dir = dirs::cache_dir().map(|d| d.join("musicTUI"));
                            match AudioPlayer::new(&cmd.token, cache_dir).await {
                                Ok((p, rx, spec)) => {
                                    player = Some(p);
                                    event_rx = Some(rx);
                                    spectrum = Some(spec);
                                }
                                Err(e) => {
                                    emit(EventOut {
                                        event: "error".into(),
                                        message: Some(format!("Failed to init player: {}", e)),
                                        ..Default::default()
                                    });
                                    continue;
                                }
                            }
                        }
                        if let Some(ref p) = player {
                            emit(EventOut { event: "loading".into(), track_id: Some(cmd.track_id.clone()), ..Default::default() });
                            if let Err(e) = p.load_track(&cmd.track_id, true, 0) {
                                emit(EventOut { event: "error".into(), message: Some(e.to_string()), ..Default::default() });
                            }
                        }
                    }
                    "pause" => {
                        if let Some(ref p) = player { p.pause(); }
                    }
                    "resume" => {
                        if let Some(ref p) = player { p.play(); }
                    }
                    "stop" => {
                        if let Some(ref p) = player { p.stop(); }
                    }
                    "seek" => {
                        if let Some(ref p) = player { p.seek(cmd.position_ms); }
                    }
                    "volume" => {
                        if let Some(ref p) = player { p.set_volume_percent(cmd.value); }
                    }
                    _ => {}
                }
            }
            Some(ev) = async { match event_rx.as_mut() { Some(rx) => rx.recv().await, None => std::future::pending().await } } => {
                match ev {
                    AudioEvent::Loading => emit(EventOut { event: "loading".into(), ..Default::default() }),
                    AudioEvent::Playing { position_ms } => emit(EventOut { event: "playing".into(), position_ms: Some(position_ms), ..Default::default() }),
                    AudioEvent::Paused { position_ms } => emit(EventOut { event: "paused".into(), position_ms: Some(position_ms), ..Default::default() }),
                    AudioEvent::PositionChanged { position_ms } => emit(EventOut { event: "position".into(), position_ms: Some(position_ms), ..Default::default() }),
                    AudioEvent::Stopped => emit(EventOut { event: "stopped".into(), ..Default::default() }),
                    AudioEvent::EndOfTrack => emit(EventOut { event: "end_of_track".into(), ..Default::default() }),
                    AudioEvent::Unavailable { track_id } => emit(EventOut { event: "error".into(), message: Some(format!("Unavailable: {}", track_id)), ..Default::default() }),
                }
            }
            else => break,
        }
    }
}

impl Default for EventOut {
    fn default() -> Self {
        Self {
            event: String::new(),
            track_id: None,
            position_ms: None,
            message: None,
            magnitudes: None,
            bass: None,
            mids: None,
            highs: None,
            energy: None,
            beat: None,
        }
    }
}
