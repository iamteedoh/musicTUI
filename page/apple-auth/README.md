# Apple Music auth page (static)

A single HTML page that uses [MusicKit JS](https://js-cdn.music.apple.com/musickit/v3/musickit.js)
to get a Music User Token from the end-user, then POSTs it back to the
musicTUI running on their computer via a local callback URL.

## Why this exists

Apple Music's Web API requires two tokens to read a user's library:

1. **Developer Token** — a JWT signed with your Apple Developer private
   key. Generated out-of-band; lives up to 180 days.
2. **Music User Token** — obtained only via MusicKit (JS / iOS / macOS
   native). Can't be obtained from Go / a terminal.

This page is the minimum viable shim for #2: MusicKit JS in a browser,
user signs in, we receive the Music User Token and ship it to the local
TUI.

## How musicTUI uses it

When the user picks "Import from Apple Music" in musicTUI:

1. musicTUI starts a local HTTP listener on `127.0.0.1:<random port>`
   and generates a random `state` nonce.
2. musicTUI opens this page in the user's default browser with the
   URL parameters:
   - `dev=<developer-token JWT>`  (generated from your .p8 once per session, cached)
   - `cb=http://127.0.0.1:<port>/applemusic/callback`
   - `state=<nonce>`
3. Page configures MusicKit with the dev token.
4. User clicks "Sign in with Apple Music"; MusicKit opens Apple's
   official auth popup.
5. On success, MusicKit hands us a Music User Token; page POSTs
   `{music_user_token, state}` to `cb`.
6. musicTUI's local server accepts the POST iff the `state` matches,
   persists credentials, and closes the listener.

## Hosting

MusicKit JS requires HTTPS (except on `localhost`). The production
target is your doralab k3s cluster with an existing TLS ingress — the
page is one HTML file, no backend needed. See the deployment notes in
the root `README.md` under "Apple Music Setup" for the specific
manifest.

## Security posture

- The developer token is passed as a URL parameter. It's short-lived
  (minutes, not days) and scoped to this session; a leak is bounded.
- The local callback uses a per-session random `state` nonce. Other
  browser tabs, other processes, malicious extensions that POST
  something to the local port will be rejected unless they know the
  nonce.
- The Music User Token is stored at
  `~/.config/musicTUI/applemusic-credentials.json` with 0600 perms,
  same pattern as the Spotify / YouTube Music credentials.
