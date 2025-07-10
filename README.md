# Gleip

Gleip is your web security companion, crafted for ethical hackers, security researchers, and bug bounty hunters who want to go beyond basic intercepting proxies. Gleip empowers you to analyze, manipulate, and automate HTTP requests with precision, revealing vulnerabilities that hide within real-world, multi-step user flows.

Whether you’re testing complex login sequences, chained API calls, or intricate e-commerce transactions, Gleip helps you capture, replay, and mutate requests as cohesive flows, rather than as isolated HTTP requests. Extract variables, insert custom logic, and experiment with fuzzing to build powerful, repeatable Proofs of Concept.

Built with modern offensive workflows in mind, Gleip transforms tedious manual testing into streamlined processes that mirror how users (and attackers) actually navigate applications. It’s designed to help you move fast, dig deeper, and understand your targets end-to-end.

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

### Supported Platforms

- `windows/amd64`
- `windows/arm64`
- `darwin/arm64`
- `linux/amd64`
- `linux/arm64`
