# Gleip

An intercepting proxy application for ethical hackers to find and exploit vulnerabilities in web applications.

## Quick Start

### Building

Gleip uses a standardized Makefile build system:

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Build for specific platform
make build-target TARGET=windows/amd64

# Development mode
make dev

# Show available targets
make help
```

### Available Make Targets

- **build** - Build for current platform (default)
- **build-all** - Build for all supported platforms
- **build-target** - Build for specific platform (requires TARGET=platform/arch)
- **dev** - Run in development mode
- **certs** - Generate certificate files
- **save-certs** - Generate and save certificate databases for CI
- **deps** - Install platform dependencies
- **clean** - Remove build artifacts
- **targets** - Show supported build targets
- **publish** - Download CI artifacts, sign DMGs, and publish release
- **sign-dmg** - Sign a CI-built DMG (requires DMG_PATH=path/to/file.dmg)
- **help** - Show help message

### Supported Platforms

- `windows/amd64`
- `windows/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`

### Advanced Usage

For more advanced build options, you can call the build tool directly:

```bash
go run cmd/build-tool/main.go [command] [args...]
```
