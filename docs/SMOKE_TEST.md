# musicTUI Smoke Test Checklist

Use this checklist before closing reliability, onboarding, UI, playback, import, or release-readiness work. Run from a macOS terminal unless a step says otherwise.

## Automated Checks

From the repository root:

```bash
go test ./...
cd bridge
cargo test
```

If the Go build cache is blocked by a restricted environment, run:

```bash
GOCACHE=/tmp/musicTUI-go-build-cache go test ./...
```

## Build Check

```bash
make build
./dist/musicTUI
```

Expected:

* `dist/musicTUI` exists.
* The app opens in the terminal alt screen.
* If no Spotify client ID is configured, the onboarding wizard opens.
* If a client ID is configured, the app opens to Home and attempts cached auth.

## First-Run Onboarding

To test without touching your real config, use a temporary HOME:

```bash
tmp_home="$(mktemp -d)"
HOME="$tmp_home" ./dist/musicTUI
```

Expected:

* The Spotify setup wizard appears.
* Arrow keys or Enter advance steps.
* `O` on the dashboard step opens the Spotify dashboard or leaves the URL visible if browser launch fails.
* Final step rejects an empty Client ID.
* Esc skips the wizard and Home says setup is still needed.

## Existing Config Startup

Use your normal terminal environment:

```bash
./dist/musicTUI
```

Expected:

* The app starts without panics.
* Home shows either connected Spotify status or a clear re-auth/setup prompt.
* `Ctrl+L` opens the Spotify auth flow.
* `?` opens help and `?` or Esc returns to the previous view.

## Navigation and Layout

Inside the app:

* Use `j`/`k` to move through sidebar navigation.
* Use Tab and Shift+Tab to cycle Sidebar, Content, and Right panels.
* Resize the terminal narrower and wider.

Expected:

* Focus border changes are visible.
* Panel borders do not visibly break.
* Text truncates rather than spilling into adjacent panels.
* Status messages are capped to the center panel width.

## Spotify Library, Search, and Playlists

With a configured Spotify account:

* Open Library. Confirm saved tracks load and pagination continues near the bottom.
* Press `/`, search for a known artist or track, and press Enter.
* Drill into an artist, then an album, then tracks.
* Open a playlist from the sidebar and confirm tracks load.

Expected:

* Loading and error states are readable.
* Enter on a track starts queue playback.
* `a` opens the add-to-playlist picker where supported.
* Removing or moving playlist tracks asks for confirmation where destructive.

## Playback, Artwork, Lyrics, and Visualizer

With Spotify Premium and a working audio output device:

* Start a track from Library, Search, or Playlist.
* Press Space, `n`, `p`, `+`, `-`, `s`, `r`, and `l`.

Expected:

* Playback starts, pauses, resumes, and advances.
* Queue highlights the current track.
* Artwork loads or shows a graceful placeholder/error.
* Lyrics load when available and can be toggled.
* The mini visualizer moves while audio is playing.
* Playback errors point to the bridge log without exposing secrets.

Bridge log locations:

* macOS: `~/Library/Caches/musicTUI/bridge.log`
* Linux: `~/.cache/musicTUI/bridge.log`
* Windows: `%LOCALAPPDATA%\musicTUI\bridge.log`

## Import Setup and Import Flow

From the Import screen:

* With no import credentials, press Enter.
* Step through the setup wizard.
* Confirm secret fields are masked.
* Cancel with Esc and re-open setup.

Expected:

* The wizard explains Google and Spotify requirements.
* Secret values are not echoed in plain text.
* Browser-open steps still show enough instruction if browser launch fails.
* Errors mention missing credential categories, not credential values.
* Provider errors show recovery guidance. For example, a Google
  `invalid_grant` should explain that the saved YouTube login can no
  longer be refreshed and offer `r` to reconnect YouTube.

Only run a real import with test playlists or a Spotify account where new playlists are safe to create.

## Update Prompt

If Home reports an update:

* Press `Ctrl+U`.

Expected:

* The app reports download/apply progress.
* Failure states are clear and do not corrupt the current binary.

## Reporting Bugs

Create a MUS Jira ticket for confirmed runtime bugs. Include:

* Branch/commit tested.
* OS and terminal.
* Exact command used to launch.
* Reproduction steps.
* Expected and actual behavior.
* Last relevant bridge log lines if playback is involved.

Do not paste tokens, client secrets, passwords, usernames, private keys, or NAS credentials into Jira, Confluence, logs, screenshots, or commit messages. Reference vault items when credentials are relevant.
