# Contributing to Lazykamal

Thanks for considering contributing! Here's how to get started.

## Prerequisites

- **Go 1.21+** - [Install Go](https://go.dev/doc/install)
- **Kamal CLI** - [Install Kamal](https://kamal-deploy.org/docs/installation/) (required for running the TUI)
- **golangci-lint** (optional, for linting) - [Install golangci-lint](https://golangci-lint.run/usage/install/)

## Development Setup

1. **Clone the repository:**
   ```bash
   git clone https://github.com/shuvro/lazykamal.git
   cd lazykamal
   ```

2. **Download dependencies:**
   ```bash
   make deps
   # or: go mod download && go mod tidy
   ```

3. **Build:**
   ```bash
   make build
   # or: go build -o lazykamal .
   ```

4. **Run:**
   ```bash
   ./lazykamal
   # Run from a directory with config/deploy.yml
   ```

## Development Workflow

### Running Tests

```bash
# Run all tests with race detection
make test

# Run tests quickly without race detection
make test-short

# Run tests with coverage report
make coverage
```

### Linting

```bash
# Run linter
make lint

# Run linter with auto-fix
make lint-fix
```

### Formatting

```bash
# Format code
make fmt

# Run go vet
make vet
```

### Full Check (before submitting PR)

```bash
# Run all checks: format, vet, lint, test
make check
```

## Project Structure

```
├── main.go              # Entry point
├── pkg/
│   ├── gui/             # TUI implementation (gocui)
│   │   ├── gui.go       # Main GUI logic and rendering
│   │   └── editor.go    # In-TUI file editor
│   └── kamal/           # Kamal CLI wrapper
│       ├── config.go    # Deploy config discovery
│       └── runner.go    # Kamal command execution
├── .github/
│   ├── workflows/       # CI/CD workflows
│   └── ISSUE_TEMPLATE/  # Issue templates
└── scripts/             # Installation scripts
```

## Releases

Releases are built with [GoReleaser](https://goreleaser.io/). Pushing a tag like `v1.0.0` triggers the [Release workflow](.github/workflows/release.yml).

```bash
# Test release locally (no publish)
make release-snapshot
# or: goreleaser release --snapshot --clean

# Real release: tag and push
git tag v1.0.0
git push origin v1.0.0
```

## Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Run tests and linting** before submitting:
   ```bash
   make check
   ```
3. **Keep changes focused** - one feature or fix per PR
4. **Write clear commit messages** explaining what and why
5. **Update documentation** if adding new features

### PR Checklist

- [ ] Tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code is formatted (`make fmt`)
- [ ] Documentation updated if needed
- [ ] CHANGELOG.md updated for significant changes

## Adding New Kamal Commands

1. Add the command function in `pkg/kamal/runner.go`:
   ```go
   func NewCommand(opts RunOptions) (Result, error) {
       return RunKamal([]string{"new", "command"}, opts)
   }
   ```

2. Add a menu item in the appropriate `renderXxxMenu` function in `pkg/gui/gui.go`

3. Add the handler in the corresponding `execXxx` function

4. Update the menu navigation bounds in `keyDown` if needed

5. Add tests in `pkg/kamal/runner_test.go`

## Reporting Issues

- Use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md) for bugs
- Use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.md) for new features
- Include reproduction steps and your environment details

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
