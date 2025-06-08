# Gleip

Gleip is an advanced intercepting proxy designed for ethical hackers, security researchers, and bug bounty hunters. It helps you analyze, manipulate, and automate HTTP traffic in order to discover and exploit vulnerabilities in web applications, especially those that depend on complex user flows.

Whether you're testing login sequences, chained API calls, or multi-step transactions, Gleip gives you the tools to craft, replay, and mutate requests with precision.

## Quick Start

### Building

Gleip uses a standardized Makefile build system:

```bash
# Build for current platform
make build

# Development mode
make dev

# Show available targets
make help
```

### Available Make Targets

- **build** - Build for current platform (default)
- **certs** - Generate certificate files
- **save-certs** - Generate and save certificate databases for CI
- **deps** - Install platform dependencies
- **clean** - Remove build artifacts
- **publish** - Download CI artifacts, sign DMGs, and publish release
- **sign-dmg** - Sign a CI-built DMG (requires DMG_PATH=path/to/file.dmg)
- **help** - Show help message

### Supported Platforms

- `windows/amd64`
- `windows/arm64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`

### Advanced Usage

For more advanced build options, you can call the build tool directly:

```bash
go run cmd/build-tool/main.go [command] [args...]
```
