# Lazykamal

A **lazydocker-style** terminal UI for [Kamal](https://kamal-deploy.org)-deployed apps. Manage deploy, app, server, accessory, and proxy from one interactive screen—no need to remember every `kamal` command.

## Features

- **Live status** – App version and containers for the selected destination refresh every few seconds
- **Live logs** – Stream app or proxy logs in real time; press Esc to stop
- **Animated spinner** – Visual feedback with spinning animation while commands run
- **Command timing** – See exactly how long each command takes to complete
- **Timestamped logs** – Every log entry shows when it happened
- **Confirmation dialogs** – Safety prompts before destructive operations (rollback, remove, stop)
- **Breadcrumb navigation** – Always know where you are in the app
- **Color-coded output** – Green ✓ for success, red ✗ for errors, yellow ● for running
- **Help overlay** – Press `?` anytime to see all keyboard shortcuts
- **In-TUI editor** – Edit deploy.yml and secrets without leaving the app

### Why Lazykamal?

- **Single VPS, many apps**: Discover `config/deploy*.yml` destinations and run any Kamal command per app
- **All Kamal commands**: Deploy, redeploy, rollback, app, server, accessory, proxy, and more
- **Written in Go** (like [lazydocker](https://github.com/jesseduffield/lazydocker)), using [gocui](https://github.com/jroimartin/gocui)

## Requirements

- [Kamal](https://kamal-deploy.org/docs/installation/) installed and on your `PATH`
- Go 1.21+ (only if building from source)

## Installation

### Homebrew (macOS / Linux)

If the formula is in Homebrew core:

```bash
brew install lazykamal
```

Or tap this repo (for a frequently updated formula):

```bash
brew tap lazykamal/lazykamal https://github.com/lazykamal/homebrew-lazykamal
brew install lazykamal
```

Or install from the local formula (after cloning):

```bash
brew install --build-from-source ./Formula/lazykamal.rb
```

### Go install

```bash
go install github.com/shuvro/lazykamal@latest
```

Ensure `$GOPATH/bin` or `$HOME/go/bin` is in your `PATH`.

### Install script (Linux / macOS)

Downloads the latest release and installs to `~/.local/bin` (override with `DIR`):

```bash
curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install_update_linux.sh | bash
```

Custom install directory:

```bash
DIR=/usr/local/bin bash -c "$(curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install_update_linux.sh)"
```

### Scoop (Windows)

```powershell
scoop bucket add lazykamal https://github.com/shuvro/scoop-lazykamal
scoop install lazykamal
```

*(Requires creating [scoop-lazykamal](https://github.com/shuvro/scoop-lazykamal) repo first - see below)*

### Chocolatey (Windows)

```bash
choco install lazykamal
```

*(Requires the package to be published to [Chocolatey](https://chocolatey.org).)*

### Binary release

Download the latest [release](https://github.com/shuvro/lazykamal/releases) for your OS and architecture (e.g. `lazykamal_1.0.0_Linux_amd64.tar.gz`), extract, and put `lazykamal` in your `PATH`.

### Build from source

```bash
git clone https://github.com/shuvro/lazykamal.git
cd lazykamal
go build -o lazykamal .
./lazykamal
```

## Usage

Run from a directory that contains Kamal config (e.g. `config/deploy.yml` or `config/deploy.staging.yml`):

```bash
lazykamal
```

Optional: pass a working directory:

```bash
lazykamal /path/to/your/kamal-app
```

### Keybindings

| Key        | Action                    |
|-----------|----------------------------|
| **↑ / ↓** | Move selection             |
| **Enter** | Open menu / Run command    |
| **m**     | Open main command menu     |
| **b** / **Esc** | Back (or stop live logs) |
| **r**     | Refresh destinations & status |
| **c**     | Clear output/log panel     |
| **?**     | Show help overlay          |
| **q**     | Quit                       |

### Screens

1. **Apps** – List of deploy destinations (`config/deploy*.yml`). Select one and press Enter to open the command menu.
2. **Main menu** – Deploy, App, Server, Accessory, Proxy, Other, **Config**.
3. **Submenus** – Deploy, App (includes **Live: App logs**), Server, Accessory, Proxy (includes **Live: Proxy logs**), Other, **Config** (edit deploy config, edit secrets, redeploy, app restart).
4. **Live status** (top right) – Auto-refreshes every few seconds: app version and containers for the selected destination.
5. **Output / Live logs** (bottom right) – Last command output, or **streaming** app/proxy logs when you run “Live: App logs” or “Live: Proxy logs”. Press **Esc** to stop streaming.

### Config (edit and restart)

From **Config** in the main menu you can:

- **Edit deploy config (current dest)** – Opens the selected app’s `config/deploy.yml` (or `config/deploy.<dest>.yml`) in an **in-TUI editor** (nano/vi style). No external editor needed—works on servers too.
- **Edit secrets (current dest)** – Opens `.kamal/secrets` (or `.kamal/secrets.<dest>`) in the same in-TUI editor. Creates `.kamal` and the secrets file if missing.
- **Redeploy (after edit)** – Runs `kamal redeploy` for the selected destination.
- **App restart (after edit)** – Runs `kamal app restart` for the selected destination.

**In-TUI editor (nano/vi style):** A full-screen modal inside the TUI. **Arrow keys** move, **typing** inserts, **Enter** newline, **Backspace** delete. **^S** (Ctrl+S) save, **^Q** or **Esc** quit (prompts if unsaved). No `$EDITOR`, nano, or vim required—ideal when Lazykamal runs on a server.

Supports both `config/deploy*.yml` and `config/deploy*.yaml`.

## Kamal command coverage

Lazykamal exposes **all** Kamal CLI commands via the TUI:

| Category | Commands |
|----------|----------|
| **Deploy** | deploy, deploy (skip push), redeploy, rollback, setup |
| **App** | boot, start, stop, restart, logs, containers, details, images, version, stale_containers, exec (e.g. whoami), maintenance, live, remove |
| **Server** | bootstrap, exec (date, uptime) |
| **Accessory** | boot/start/stop/restart/reboot/remove/details/logs all, upgrade |
| **Proxy** | boot, start, stop, restart, reboot, reboot (rolling), logs, details, remove, boot_config get/set/reset |
| **Other** | prune, build, config, details, audit, lock (status/acquire/release/release --force), registry (login/logout), secrets, env (push/pull/delete), docs, help, init, upgrade, version |

Top-level Kamal commands **build**, **registry**, **secrets**, **docs**, **help**, **init**, and **upgrade** are under **Other**. Options like `--primary`, `--hosts`, `--roles`, `--version` are passed via the selected destination (config file and destination name); future versions may expose them in the UI.

## Comparison with lazydocker

Lazykamal is inspired by [lazydocker](https://github.com/jesseduffield/lazydocker) and aims for similar ergonomics, but the domain is different (Kamal vs Docker).

| Aspect | Lazydocker | Lazykamal |
|--------|------------|-----------|
| **Domain** | Docker / Compose (containers, images, volumes, networks) | Kamal (deploy, app, server, accessory, proxy) |
| **Tech** | Go + gocui | Go + gocui |
| **Install** | Homebrew, Scoop, Chocolatey, go install, script, AUR, Docker | Homebrew, go install, script, binary (same style) |
| **Navigation** | Panels + keybindings, mouse support | Panels + keybindings (↑/↓, Enter, m, b, q) |
| **Actions** | Start/stop/restart/remove, view logs, attach, custom commands | Run any Kamal command from menus; output in right panel |
| **Live data** | Real-time container logs, CPU/memory stats, list of containers/images | **Live status** (polled app version + containers); **streaming logs** (app, proxy) |
| **Multi-context** | Switch Docker context | Select deploy destination (config/deploy*.yml) per app |

**On par with lazydocker:** Go + gocui, install options, keyboard-driven TUI, one place to run all relevant commands, **live status panel**, **streaming logs** (Esc to stop), **animated spinners**, **command timing**, and **confirmation dialogs** for destructive actions.

## Development

```bash
# Build
make build          # Build binary
make run            # Build and run

# Test
make test           # Run tests with race detection
make test-short     # Run tests quickly
make coverage       # Run tests and open coverage report

# Lint
make lint           # Run golangci-lint
make lint-fix       # Run linter with auto-fix
make fmt            # Format code

# All checks before PR
make check          # Run fmt, vet, lint, test

# Release
make release-snapshot  # Test release build (no publish)
```

Or without Make:
- `go build .` – build binary
- `go run .` – run from source
- `go test -v ./...` – run tests
- `goreleaser release --snapshot --clean` – build archives for all platforms

## License

[MIT](LICENSE). Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).
