APP_NAME    := musictui
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -s -w
DIST        := dist
BRIDGE_BIN  := player-bridge

.PHONY: build build-linux build-macos build-windows build-bridge clean dist all

# ── Default: build for current platform ──────────────────────
build: build-bridge
	@mkdir -p $(DIST)
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(APP_NAME) .
	cp bridge/target/release/$(BRIDGE_BIN) $(DIST)/$(BRIDGE_BIN)
	@echo "Built: $(DIST)/$(APP_NAME) + $(DIST)/$(BRIDGE_BIN)"

# ── Build Rust player-bridge ─────────────────────────────────
build-bridge:
	cd bridge && cargo build --bin $(BRIDGE_BIN) --release

# ── Platform-specific Go builds ──────────────────────────────
build-linux:
	@mkdir -p $(DIST)/linux-amd64
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/linux-amd64/$(APP_NAME) .
	@echo "Built: $(DIST)/linux-amd64/$(APP_NAME)"

build-macos:
	@mkdir -p $(DIST)/darwin-arm64 $(DIST)/darwin-amd64
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/darwin-arm64/$(APP_NAME) .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/darwin-amd64/$(APP_NAME) .
	@echo "Built: $(DIST)/darwin-arm64/$(APP_NAME) + $(DIST)/darwin-amd64/$(APP_NAME)"

build-windows:
	@mkdir -p $(DIST)/windows-amd64
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/windows-amd64/$(APP_NAME).exe .
	@echo "Built: $(DIST)/windows-amd64/$(APP_NAME).exe"

# ── Package release tarballs ─────────────────────────────────
dist: build-linux build-macos build-windows
	@echo "Packaging..."
	@cd $(DIST)/linux-amd64 && tar czf ../$(APP_NAME)-linux-amd64.tar.gz $(APP_NAME)
	@cd $(DIST)/darwin-arm64 && tar czf ../$(APP_NAME)-darwin-arm64.tar.gz $(APP_NAME)
	@cd $(DIST)/darwin-amd64 && tar czf ../$(APP_NAME)-darwin-amd64.tar.gz $(APP_NAME)
	@cd $(DIST)/windows-amd64 && zip -q ../$(APP_NAME)-windows-amd64.zip $(APP_NAME).exe
	@echo "Packages in $(DIST)/"
	@ls -lh $(DIST)/*.tar.gz $(DIST)/*.zip 2>/dev/null

# ── Build everything for current platform ────────────────────
all: build

# ── Install to ~/.local/bin ──────────────────────────────────
install: build
	@mkdir -p $(HOME)/.local/bin
	cp $(DIST)/$(APP_NAME) $(HOME)/.local/bin/$(APP_NAME)
	@if [ -f "$(DIST)/$(BRIDGE_BIN)" ]; then \
		cp $(DIST)/$(BRIDGE_BIN) $(HOME)/.local/bin/$(BRIDGE_BIN); \
	fi
	@echo "Installed to ~/.local/bin/"

clean:
	rm -rf $(DIST)
