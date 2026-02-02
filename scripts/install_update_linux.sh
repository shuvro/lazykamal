#!/bin/bash
# Install or update lazykamal on Linux/macOS.
# Usage: curl -sSL https://raw.githubusercontent.com/lazykamal/lazykamal/main/scripts/install_update_linux.sh | bash
# Or: DIR=/usr/local/bin bash install_update_linux.sh

set -e
DIR="${DIR:-$HOME/.local/bin}"
REPO="${REPO:-lazykamal/lazykamal}"

# Map architecture to release asset suffix
ARCH=$(uname -m)
case $ARCH in
  i386|i686) ARCH=x86 ;;
  armv6*)    ARCH=armv6 ;;
  armv7*)    ARCH=armv7 ;;
  aarch64*)  ARCH=arm64 ;;
esac

OS=$(uname -s)
case $OS in
  Linux)  OS=Linux ;;
  Darwin) OS=Darwin ;;
  *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

LATEST=$(curl -sSL -H 'Accept: application/json' "https://github.com/${REPO}/releases/latest" | sed -e 's/.*"tag_name":"\([^"]*\)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "Could not find latest release. Try setting REPO=owner/lazykamal" >&2
  exit 1
fi

FILE="lazykamal_${LATEST#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${FILE}"
echo "Installing lazykamal ${LATEST} to ${DIR}..."
mkdir -p "$DIR"
curl -sSL -o /tmp/lazykamal.tar.gz "$URL"
tar -xzf /tmp/lazykamal.tar.gz -C /tmp
install -m 755 /tmp/lazykamal "$DIR/lazykamal"
rm -f /tmp/lazykamal /tmp/lazykamal.tar.gz
echo "Installed: $DIR/lazykamal"
if ! echo "$PATH" | grep -q "$DIR"; then
  echo "Add to PATH: export PATH=\"$DIR:\$PATH\""
fi
