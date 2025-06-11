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

# Development mode
.PHONY: dev
dev: check-build-tool
	@echo "Starting Gleip in development mode..."
	@if [ -f ".env" ]; then \
		echo "Loading environment variables from .env file..."; \
		env $$(grep -v '^#' .env | grep -v '^$$' | xargs) go run $(BUILD_TOOL) dev; \
	else \
		go run $(BUILD_TOOL) dev; \
	fi

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

# Package .app as DMG (macOS only)
.PHONY: package-dmg
package-dmg: check-build-tool
	@echo "Packaging .app as signed DMG..."
	go run $(BUILD_TOOL) package-dmg

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
	@echo "  dev          - Run in development mode"
	@echo "  certs        - Generate certificate files"
	@echo "  save-certs   - Generate and save certificate databases for CI"
	@echo "  deps         - Install platform dependencies"
	@echo "  clean        - Remove build artifacts"
	@echo "  package-dmg  - Package .app as signed DMG (macOS only)"
	@echo "  publish      - Download CI artifacts, sign DMGs, and publish release"
	@echo "  sign-dmg     - Sign a CI-built DMG (requires DMG_PATH=path/to/file.dmg)"
	@echo "  help         - Show this help message"
	@echo ""
	@echo "For advanced usage, you can also call the build tool directly:"
	@echo "  go run $(BUILD_TOOL) [command] [args...]"
	@echo ""
	@go run $(BUILD_TOOL) help

# Backward compatibility: allow direct passthrough of arguments
# This ensures GitHub Actions can still call specific commands
.PHONY: build-tool
build-tool: check-build-tool
	go run $(BUILD_TOOL) $(filter-out $@,$(MAKECMDGOALS))

# Handle unknown targets by passing them to the build tool
%:
	@$(MAKE) --no-print-directory build-tool $(MAKECMDGOALS) 