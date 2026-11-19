#!/usr/bin/env sh
set -eu

REPO="${WORLD_CLI_REPO:-bootun/world-cli}"
INSTALL_DIR="${WORLD_INSTALL_DIR:-$HOME/.local/bin}"

mkdir -p "$INSTALL_DIR"

if command -v go >/dev/null 2>&1; then
  echo "Installing world via go install..."
  GOBIN="$INSTALL_DIR" go install "github.com/${REPO}/cmd/world@latest"
  echo "Installed: ${INSTALL_DIR}/world"
  case ":$PATH:" in
    *":${INSTALL_DIR}:"*) ;;
    *) echo "Add ${INSTALL_DIR} to your PATH to run 'world' directly." ;;
  esac
  exit 0
fi

echo "Go toolchain not found. Install Go 1.27+ or download a prebuilt binary from https://github.com/${REPO}/releases" >&2
exit 1
