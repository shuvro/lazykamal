# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- CI workflow for automated testing and linting on PRs
- golangci-lint configuration for consistent code quality
- Makefile with common development targets
- Issue and PR templates for better contribution workflow
- Unit tests for `pkg/kamal/config.go`, `pkg/kamal/runner.go`, and `pkg/gui/styles.go`
- Kamal installation check at startup with helpful error message
- Environment commands (`kamal env push`, `kamal env pull`, `kamal env delete`)
- Help overlay accessible via `?` key
- `--help` flag for command-line help
- Version display in TUI header with rocket icon
- Refresh shortcut (`r`) to reload destinations and status
- Clear log shortcut (`c`) to clear the output panel
- **Animated loading spinner** for running commands
- **Command execution timing** - shows duration for each command
- **Timestamped log entries** for better tracking
- **Confirmation dialogs** for destructive operations (rollback, remove, stop, prune, etc.)
- **Breadcrumb navigation** showing current location in the app
- **Color-coded output** with success/error/info indicators
- **Status indicators** in header showing Ready/Running state
- Graceful shutdown handling for SIGINT/SIGTERM signals
- CHANGELOG.md for tracking changes

### Changed
- Secrets file permissions now use 0600 for better security
- Updated CONTRIBUTING.md with testing and linting workflows
- Improved .gitignore with more comprehensive patterns
- Updated README with new keyboard shortcuts and env commands
- Refactored command execution to use consistent pattern with spinner and timing
- Improved visual styling with Unicode icons and ANSI colors

### Security
- Added path traversal protection for file editing operations
- Added symlink attack detection and prevention
- Changed `.kamal` directory permissions from 0755 to 0700
- Secrets files created with 0600 permissions
- Added sensitive data sanitization for log output (passwords, tokens, API keys)
- Added working directory validation to prevent directory traversal attacks
- Added security utility functions with comprehensive tests

### Fixed
- SecretsPath now correctly returns destination-specific path for non-production environments
- Improved error handling throughout the codebase with colored error messages

## [0.1.0] - Initial Release

### Added
- Terminal UI for Kamal deployments using gocui
- Support for multiple deploy destinations (production, staging, etc.)
- Deploy, redeploy, rollback, and setup commands
- App management (boot, start, stop, restart, logs, exec)
- Server management (bootstrap, exec)
- Accessory management (boot, start, stop, restart, logs)
- Proxy management (boot, start, stop, restart, logs)
- Live log streaming for app, proxy, and accessories
- In-TUI file editor for deploy.yml and secrets
- Lock management (acquire, release, status)
- Build, prune, and audit commands
- Registry login/logout support
