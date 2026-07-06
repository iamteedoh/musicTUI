# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
