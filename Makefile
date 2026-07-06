APP_NAME    := musicTUI
LDFLAGS     := -s -w
DIST        := dist
BRIDGE_BIN  := player-bridge

.PHONY: build build-bridge clean install test test-go test-bridge

# ── Default: build single binary with embedded bridge ────
build: build-bridge
	@mkdir -p bridge-bin
	cp bridge/target/release/$(BRIDGE_BIN) bridge-bin/
	@mkdir -p $(DIST)
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME) .
	@echo "Built: $(DIST)/$(APP_NAME) (bridge embedded)"

# ── Build Rust player-bridge ─────────────────────────────
build-bridge:
	cd bridge && cargo build --bin $(BRIDGE_BIN) --release

test: test-go test-bridge

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
	rm -rf $(DIST) bridge-bin/$(BRIDGE_BIN)
