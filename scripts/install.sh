#!/bin/bash
#
# Install script for lazykamal
# Usage: curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install.sh | bash
#
# Options (via environment variables):
#   DIR       - Installation directory (default: ~/.local/bin or /usr/local/bin if root)
#   VERSION   - Specific version to install (default: latest)
#
# Examples:
#   curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install.sh | bash
#   curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install.sh | DIR=/usr/local/bin bash
#   curl -sSL https://raw.githubusercontent.com/shuvro/lazykamal/main/scripts/install.sh | VERSION=v1.0.3 bash

set -e

REPO="shuvro/lazykamal"
BINARY_NAME="lazykamal"
GITHUB_URL="https://github.com/${REPO}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1" >&2; exit 1; }

# Detect OS
detect_os() {
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux*)  echo "linux" ;;
        darwin*) echo "darwin" ;;
        mingw*|msys*|cygwin*) echo "windows" ;;
        *)       error "Unsupported OS: $os" ;;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7*)        echo "arm" ;;
        *)             error "Unsupported architecture: $arch" ;;
    esac
}

# Get latest version from GitHub
get_latest_version() {
    curl -sSL -H 'Accept: application/json' "${GITHUB_URL}/releases/latest" 2>/dev/null | \
        sed -e 's/.*"tag_name":"\([^"]*\)".*/\1/' || \
        error "Failed to fetch latest version"
}

# Determine install directory
get_install_dir() {
    if [ -n "$DIR" ]; then
        echo "$DIR"
    elif [ "$(id -u)" = "0" ]; then
        echo "/usr/local/bin"
    else
        echo "${HOME}/.local/bin"
    fi
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Main installation
main() {
    echo ""
    echo "  _                 _  __                       _ "
    echo " | |    __ _ _____  | |/ /__ _ _ __ ___   __ _  | |"
    echo " | |   / _\` |_  / | | ' // _\` | '_ \` _ \\ / _\` | | |"
    echo " | |__| (_| |/ /| |_| . \\ (_| | | | | | | (_| | | |"
    echo " |_____\\__,_/___|\\__, |_|\\_\\__,_|_| |_| |_|\\__,_| |_|"
    echo "                 |___/                              "
    echo ""
    echo " Lazydocker-style TUI for Kamal deployments"
    echo ""

    # Check for required tools
    if ! command_exists curl; then
        error "curl is required but not installed"
    fi

    if ! command_exists tar; then
        error "tar is required but not installed"
    fi

    # Detect system
    local os arch version install_dir
    os=$(detect_os)
    arch=$(detect_arch)
    
    info "Detected: ${os}/${arch}"

    # Check Windows
    if [ "$os" = "windows" ]; then
        error "Windows detected. Please use Scoop or download the .zip from ${GITHUB_URL}/releases"
    fi

    # Get version
    if [ -n "$VERSION" ]; then
        version="$VERSION"
    else
        info "Fetching latest version..."
        version=$(get_latest_version)
    fi

    if [ -z "$version" ]; then
        error "Could not determine version to install"
    fi

    info "Version: ${version}"

    # Determine install directory
    install_dir=$(get_install_dir)
    info "Install directory: ${install_dir}"

    # Build download URL
    local version_num="${version#v}"
    local filename="${BINARY_NAME}_${version_num}_${os}_${arch}.tar.gz"
    local download_url="${GITHUB_URL}/releases/download/${version}/${filename}"

    info "Downloading ${filename}..."

    # Create temp directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap "rm -rf ${tmp_dir}" EXIT

    # Download
    if ! curl -sSL -o "${tmp_dir}/${filename}" "$download_url"; then
        error "Failed to download from ${download_url}"
    fi

    # Extract
    info "Extracting..."
    tar -xzf "${tmp_dir}/${filename}" -C "${tmp_dir}"

    # Install
    mkdir -p "$install_dir"
    
    if [ -w "$install_dir" ]; then
        mv "${tmp_dir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
        chmod +x "${install_dir}/${BINARY_NAME}"
    else
        info "Requesting sudo to install to ${install_dir}..."
        sudo mv "${tmp_dir}/${BINARY_NAME}" "${install_dir}/${BINARY_NAME}"
        sudo chmod +x "${install_dir}/${BINARY_NAME}"
    fi

    success "Installed ${BINARY_NAME} to ${install_dir}/${BINARY_NAME}"

    # Check PATH
    if ! echo "$PATH" | tr ':' '\n' | grep -q "^${install_dir}$"; then
        echo ""
        warn "${install_dir} is not in your PATH"
        echo ""
        echo "Add it to your shell profile:"
        echo ""
        echo "  # For bash (~/.bashrc or ~/.bash_profile):"
        echo "  export PATH=\"${install_dir}:\$PATH\""
        echo ""
        echo "  # For zsh (~/.zshrc):"
        echo "  export PATH=\"${install_dir}:\$PATH\""
        echo ""
    fi

    # Verify installation
    if command_exists "$BINARY_NAME"; then
        echo ""
        success "Installation complete!"
        echo ""
        "${install_dir}/${BINARY_NAME}" --version 2>/dev/null || true
        echo ""
        echo "Run '${BINARY_NAME}' from a directory with Kamal config (config/deploy.yml)"
        echo ""
    else
        echo ""
        success "Installation complete!"
        echo ""
        echo "Run '${install_dir}/${BINARY_NAME}' from a directory with Kamal config"
        echo ""
    fi
}

main "$@"
