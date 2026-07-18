# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.2](https://github.com/iamteedoh/musicTUI/compare/v0.3.1...v0.3.2) (2026-07-18)


### Features

* add peak-hold caps and keep visualizer motion on tempo ([#23](https://github.com/iamteedoh/musicTUI/issues/23)) ([1c33f05](https://github.com/iamteedoh/musicTUI/commit/1c33f0584c8c86eb7fd9e3c62f02e98589653646))


### Bug Fixes

* adapt theme text contrast to the terminal background so it stays legible ([#29](https://github.com/iamteedoh/musicTUI/issues/29)) ([d5fab75](https://github.com/iamteedoh/musicTUI/commit/d5fab75d0198828882cbdf36a15aef15556cf3a0))
* match the theme to the terminal's background ([#25](https://github.com/iamteedoh/musicTUI/issues/25)) ([1ab2023](https://github.com/iamteedoh/musicTUI/commit/1ab2023e006bf975a90088d8d4e7f1f121d7096b))
* prevent blank kitty artwork panel caused by float truncation in image-id color encoding ([#28](https://github.com/iamteedoh/musicTUI/issues/28)) ([4a85ff3](https://github.com/iamteedoh/musicTUI/commit/4a85ff301ae9742983bf7750dd67371786cccf04))
* render album artwork in iTerm2 ([#26](https://github.com/iamteedoh/musicTUI/issues/26)) ([a0b5403](https://github.com/iamteedoh/musicTUI/commit/a0b5403cf211b2403277abf05265298318b0f05e))
* replace the raw lyrics fetch error with a retryable message ([#27](https://github.com/iamteedoh/musicTUI/issues/27)) ([7d840cd](https://github.com/iamteedoh/musicTUI/commit/7d840cd36006a85473087374f54032c935064b4d))

## [0.3.1](https://github.com/iamteedoh/musicTUI/compare/v0.3.0...v0.3.1) (2026-07-09)


### Bug Fixes

* detect kitty graphics by querying the terminal; add --version ([#19](https://github.com/iamteedoh/musicTUI/issues/19)) ([4a570b2](https://github.com/iamteedoh/musicTUI/commit/4a570b23beb51b18afda1d419b4c628a73ca44f7))
* ignore Windows modifier key-downs in TUI text fields ([#21](https://github.com/iamteedoh/musicTUI/issues/21)) ([cc01eb6](https://github.com/iamteedoh/musicTUI/commit/cc01eb6c3a83a3321620b2764063b7210216c311))
* render album artwork as real pixels in terminals without kitty graphics ([#22](https://github.com/iamteedoh/musicTUI/issues/22)) ([a211ee3](https://github.com/iamteedoh/musicTUI/commit/a211ee398b2f125d052d1b7cd8da9fd7511126d6))

## [0.3.0](https://github.com/iamteedoh/musicTUI/compare/v0.2.0...v0.3.0) (2026-07-07)


### Features

* rename Library to "Liked Songs" + document Spotify playlist visibility ([#11](https://github.com/iamteedoh/musicTUI/issues/11)) ([c352c1b](https://github.com/iamteedoh/musicTUI/commit/c352c1bfa758ffbde19f33700c3d8e5d92593dec))
* render album artwork as solid half-block cells (MUS-15) ([#8](https://github.com/iamteedoh/musicTUI/issues/8)) ([e6ac637](https://github.com/iamteedoh/musicTUI/commit/e6ac637ca59aa38651112f69ff663e66932ebce6))


### Bug Fixes

* bridge death recovery + state-clobber race on track switch (MUS-17) ([#10](https://github.com/iamteedoh/musicTUI/issues/10)) ([8e9ddfc](https://github.com/iamteedoh/musicTUI/commit/8e9ddfcae0ef1cf8e28f20f78967ce8fc4c47ba2))
* clear rate-limit skip message + rune-safe error truncation ([#17](https://github.com/iamteedoh/musicTUI/issues/17)) ([22d047c](https://github.com/iamteedoh/musicTUI/commit/22d047c7b33baca614a9f1078de6716be165d5dd))
* Import screen keys were swallowed by the playback bindings ([#13](https://github.com/iamteedoh/musicTUI/issues/13)) ([bca0d5c](https://github.com/iamteedoh/musicTUI/commit/bca0d5cce6b1d496767b33bdcaf1354400b78be8))
* invalidate reuse-path import token when the playback client id changes ([#16](https://github.com/iamteedoh/musicTUI/issues/16)) ([21a3fb3](https://github.com/iamteedoh/musicTUI/commit/21a3fb3ff7140a38e4254f5cc79572dc8cf35f9e))
* migrate to Spotify Feb 2026 Development Mode endpoints (MUS-11) ([#9](https://github.com/iamteedoh/musicTUI/issues/9)) ([add312d](https://github.com/iamteedoh/musicTUI/commit/add312d6bbeaecb62f9a938955e9aa8e58185a32))
* reject wrong-artist matches in library import (MUS-18) ([#12](https://github.com/iamteedoh/musicTUI/issues/12)) ([2757ce9](https://github.com/iamteedoh/musicTUI/commit/2757ce913fe3a26f22cb9f16605c995d6de23de8))
* secret fields start empty on wizard re-runs; Ctrl+U clears a field ([#15](https://github.com/iamteedoh/musicTUI/issues/15)) ([ab9f427](https://github.com/iamteedoh/musicTUI/commit/ab9f4272d1b9991e34d377a568c0771694da3570))
* stop playlists doubling on background re-fetch (MUS-13) ([#3](https://github.com/iamteedoh/musicTUI/issues/3)) ([0be6b42](https://github.com/iamteedoh/musicTUI/commit/0be6b428b9814b88ffb29f85e1d716dc07ee463e))

## [Unreleased]

### Added
- In-app recovery from a wrong/rejected Spotify Client ID: `Ctrl+O` reopens the
  setup wizard (pre-filled) from any state, with actionable guidance on the
  waiting/timeout screens. The paste-Client-ID step now names and auto-opens the
  Spotify Developer Dashboard.
- Standalone `cmd/musictui-import` CLI for the library-import engine.
- Contributor docs and CI: `CONTRIBUTING`, `CODE_OF_CONDUCT`, `SECURITY`,
  issue/PR templates, and a GitHub Actions pipeline (build/test, secret scan,
  community-files check).

### Changed
- The library-import engine is now vendored into this repository under
  `internal/importcore/` (previously a separate module), so the project builds
  from a single clean checkout with no private dependencies.
- Confirmation modals word-wrap their message and center the key legend.

### Fixed
- A failed Spotify login no longer leaves the `:8888` callback server bound,
  which previously blocked re-authentication for up to two minutes.

## Earlier releases

Releases `v0.1.0` through `v0.2.0` predate this changelog. See the
[GitHub releases](https://github.com/iamteedoh/musicTUI/releases) and the git
tags for their history.

[Unreleased]: https://github.com/iamteedoh/musicTUI/compare/v0.2.0...HEAD
