# Apple Music auth page (static)

> Status: reference/experimental. Apple Music import is not currently wired
> into the musicTUI Go TUI. The active import flow is YouTube Music to
> Spotify. Keep this page as prior research unless a future MUS ticket revives
> Apple Music support.

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

## Intended flow if Apple Music support is revived

If a future ticket restores Apple Music import:

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

## Hosting notes

MusicKit JS requires HTTPS except on `localhost`. The page is one HTML
file and does not need its own backend.

## Security posture

- Never store Apple developer credentials, private keys, developer
  tokens, Music User Tokens, usernames, passwords, or other secrets in
  documentation, tickets, screenshots, or logs. Reference the vault item
  instead.
- If the developer token is passed as a URL parameter in a revived flow,
  keep it short-lived and scoped to the session.
- The local callback uses a per-session random `state` nonce. Other
  browser tabs, other processes, malicious extensions that POST
  something to the local port will be rejected unless they know the
  nonce.
- Store Apple Music credentials under the OS user config directory with
  restrictive file permissions if this feature is implemented again.
