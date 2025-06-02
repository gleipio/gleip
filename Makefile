# Gleip Build System Makefile
# Provides a standard make interface while leveraging the unified Go build tool

# Default target
.DEFAULT_GOAL := build

# Build tool path
BUILD_TOOL := cmd/build-tool/main.go

# Check if build tool exists
.PHONY: check-build-tool
check-build-tool:
	@if [ ! -f "$(BUILD_TOOL)" ]; then \
		echo "Error: Build tool not found at $(BUILD_TOOL)"; \
		exit 1; \
	fi

# Build the application (default target)
.PHONY: build
build: check-build-tool
	@echo "Building Gleip for current platform..."
	go run $(BUILD_TOOL) build

# Build for all platforms
.PHONY: build-all
build-all: check-build-tool
	@echo "Building Gleip for all platforms..."
	go run $(BUILD_TOOL) build-all

# Development mode
.PHONY: dev
dev: check-build-tool
	@echo "Starting Gleip in development mode..."
	go run $(BUILD_TOOL) dev

# Generate certificates
.PHONY: certs
certs: check-build-tool
	@echo "Generating certificates..."
	go run $(BUILD_TOOL) certs

# Save certificates for CI
.PHONY: save-certs
save-certs: check-build-tool
	@echo "Generating and saving certificate databases..."
	go run $(BUILD_TOOL) save-certs

# Install dependencies
.PHONY: deps
deps: check-build-tool
	@echo "Installing dependencies..."
	go run $(BUILD_TOOL) deps

# Clean build artifacts
.PHONY: clean
clean: check-build-tool
	@echo "Cleaning build artifacts..."
	go run $(BUILD_TOOL) clean

# Show build targets
.PHONY: targets
targets: check-build-tool
	@echo "Showing supported build targets..."
	go run $(BUILD_TOOL) targets

# Publish release (download CI artifacts, sign DMGs, and publish)
.PHONY: publish
publish: check-build-tool
	@echo "Publishing release..."
	go run $(BUILD_TOOL) publish

# Sign a CI-built DMG with local certificate
.PHONY: sign-dmg
sign-dmg: check-build-tool
ifndef DMG_PATH
	@echo "Error: DMG_PATH variable must be specified"
	@echo "Usage: make sign-dmg DMG_PATH=path/to/file.dmg"
	@exit 1
else
	@echo "Signing DMG: $(DMG_PATH)"
	go run $(BUILD_TOOL) sign-dmg $(DMG_PATH)
endif

# Show help
.PHONY: help
help: check-build-tool
	@echo "Gleip Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  build        - Build for current platform (default)"
	@echo "  build-all    - Build for all supported platforms"
	@echo "  build-target - Build for specific platform (requires TARGET=platform/arch)"
	@echo "  dev          - Run in development mode"
	@echo "  certs        - Generate certificate files"
	@echo "  save-certs   - Generate and save certificate databases for CI"
	@echo "  deps         - Install platform dependencies"
	@echo "  clean        - Remove build artifacts"
	@echo "  targets      - Show supported build targets"
	@echo "  publish      - Download CI artifacts, sign DMGs, and publish release"
	@echo "  sign-dmg     - Sign a CI-built DMG (requires DMG_PATH=path/to/file.dmg)"
	@echo "  help         - Show this help message"
	@echo ""
	@echo "For advanced usage, you can also call the build tool directly:"
	@echo "  go run $(BUILD_TOOL) [command] [args...]"
	@echo ""
	@go run $(BUILD_TOOL) help

# Support for specific build targets (e.g., make build-target TARGET=windows/amd64)
.PHONY: build-target
build-target: check-build-tool
ifdef TARGET
	@echo "Building Gleip for $(TARGET)..."
	go run $(BUILD_TOOL) build $(TARGET)
else
	@echo "Error: TARGET variable must be specified"
	@echo "Usage: make build-target TARGET=platform/arch"
	@echo "Example: make build-target TARGET=windows/amd64"
	@exit 1
endif

# Backward compatibility: allow direct passthrough of arguments
# This ensures GitHub Actions can still call specific commands
.PHONY: build-tool
build-tool: check-build-tool
	go run $(BUILD_TOOL) $(filter-out $@,$(MAKECMDGOALS))

# Handle unknown targets by passing them to the build tool
%:
	@$(MAKE) --no-print-directory build-tool $(MAKECMDGOALS) 