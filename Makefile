APP_NAME    := musicTUI
DIST        := dist

# The real build lives in ./tools/build — a Go program, so it runs identically
# on Linux, macOS and Windows (where there is no make). This Makefile is a thin
# convenience wrapper; keep the logic there so the two can't drift.
.PHONY: build clean install test test-go test-bridge

# ── Default: build single binary with embedded bridge ────
build:
	go run ./tools/build

test:
	go run ./tools/build test

test-go:
	go test ./...

test-bridge:
	cd bridge && cargo test

# ── Install to ~/.local/bin ──────────────────────────────
install: build
	@mkdir -p $(HOME)/.local/bin
	cp $(DIST)/$(APP_NAME) $(HOME)/.local/bin/$(APP_NAME)
	@echo "Installed to ~/.local/bin/$(APP_NAME)"

clean:
	go run ./tools/build clean
