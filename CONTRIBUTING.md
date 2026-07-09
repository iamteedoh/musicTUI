# Contributing to musicTUI

Thanks for your interest in improving musicTUI! This guide covers how to build,
test, and submit changes.

## Ways to contribute

- **Report a bug** — open a [bug report](https://github.com/iamteedoh/musicTUI/issues/new?template=bug_report.yml).
- **Request a feature** — open a [feature request](https://github.com/iamteedoh/musicTUI/issues/new?template=feature_request.yml).
- **Send a pull request** — see below. For anything non-trivial, please open an
  issue first so we can agree on the approach before you invest time.
- **Report a security issue** — do **not** open a public issue; see
  [SECURITY.md](SECURITY.md).

## Prerequisites

musicTUI is a Go program that embeds a small Rust audio bridge, so you need both
toolchains:

- **Go** — the version in [`go.mod`](go.mod) or newer
- **Rust** — stable toolchain (`rustup`)
- **Linux only:** ALSA headers — `sudo apt-get install libasound2-dev`
  (or your distro's equivalent)
- **Windows only:** the Visual Studio C++ build tools. Rust's default Windows
  target links with MSVC's `link.exe`, which `rustup` does **not** install:
  `winget install --id Microsoft.VisualStudio.2022.BuildTools --override "--wait --quiet --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"`
  (or `rustup default stable-gnu` to avoid Visual Studio entirely)

## Build & run

[`tools/build`](tools/build) builds the Rust bridge, embeds it into the Go
binary and stamps the version. It's a Go program, so it runs the same way on
every platform:

```sh
go run ./tools/build          # -> dist/musicTUI[.exe] (bridge embedded)
./dist/musicTUI
```

On Linux and macOS, `make build` / `make test` / `make clean` wrap it. Windows
has no `make` — call `go run ./tools/build` directly.

A bare `go build` is **not** enough: it skips the Rust bridge, so the binary
prints `Warning: player-bridge not found` and reports **No audio engine
available** at playback. Always build through `tools/build`.

## Test & lint

```sh
go run ./tools/build test   # go test ./...  +  cargo test (bridge)
go test ./...               # Go tests only
gofmt -l .                  # should print nothing; run `gofmt -w .` to fix
go vet ./...                # static checks
```

## Testing onboarding

The first-run setup wizard only opens when no `client_id` is configured. Point
`--config-dir` at an empty directory to get a clean first run without touching
your real profile:

```sh
./dist/musicTUI --config-dir /tmp/musictui-test
```

CI runs the same checks (build, `gofmt`, `go vet`, Go + Rust tests), a secret
scan (gitleaks), and a community-files check on every pull request. Please make
sure `make test` and `gofmt` are clean before opening a PR.

## Project layout

- `main.go`, `internal/tui/` — the Bubble Tea TUI (playback, onboarding, views)
- `internal/spotify/` — Spotify playback OAuth (PKCE) + Web API client
- `internal/audio/`, `bridge/` — the Go audio engine and the Rust `player-bridge`
- `internal/importcore/` — the self-contained library-import engine
  (importer, matching, OAuth loopback, YouTube/Spotify service clients)
- `internal/importbackend/` — the TUI-facing wrapper around `importcore`
- `cmd/musictui-import/` — a standalone CLI for the import engine

## Pull request process

1. Fork the repo and create a branch from `main`.
2. Make your change with tests where it makes sense.
3. Ensure `make test`, `gofmt`, and `go vet` are clean.
4. Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
   messages (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:` …).
5. Open the PR, fill out the template, and link any related issue
   (`Closes #123`).
6. A maintainer reviews once CI is green. Squash-merge is the default.

## Never commit secrets

Client secrets, OAuth tokens, and credentials must never be committed. Config
and tokens live in your OS config directory at runtime, never in the repo. CI
runs gitleaks, but please double-check your diffs.

## License

By contributing, you agree that your contributions are licensed under the
project's [GNU GPL v3](LICENSE).
