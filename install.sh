#!/usr/bin/env sh
# install.sh - install carrier from GitHub Releases
set -e

REPO="atbuy/carrier"
DEFAULT_INSTALL_DIR="${HOME}/.local/bin"
INSTALL_DIR=""
VERSION=""
USE_SYSTEM=0

usage() {
  cat <<EOF
Usage: install.sh [OPTIONS]

Install carrier from GitHub Releases.

Options:
  --version <tag>   Install a specific release (e.g. v0.1.0). Default: latest
  --system          Install to /usr/local/bin instead of ~/.local/bin (requires sudo)
  --help            Show this help message
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      if [ $# -lt 2 ]; then
        echo "Option --version requires a value." >&2
        exit 1
      fi
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      shift
      ;;
    --system)
      USE_SYSTEM=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ "${USE_SYSTEM}" = "1" ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
fi

OS="$(uname -s)"
case "${OS}" in
  Linux*) OS="linux" ;;
  Darwin*) OS="darwin" ;;
  *)
    echo "Unsupported operating system: ${OS}" >&2
    echo "This script supports Linux and macOS. For Windows, use install.ps1." >&2
    exit 1
    ;;
esac

ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: ${ARCH}" >&2
    exit 1
    ;;
esac

for tool in curl tar; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "Required tool not found: ${tool}" >&2
    exit 1
  fi
done

if [ -z "${VERSION}" ]; then
  echo "Fetching latest release version..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
  if [ -z "${VERSION}" ]; then
    echo "Failed to fetch latest release version from GitHub API." >&2
    exit 1
  fi
fi

echo "Installing carrier ${VERSION} (${OS}/${ARCH})..."
echo ""

ARCHIVE="carrier-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

mkdir -p "${INSTALL_DIR}" 2>/dev/null || {
  echo "Cannot create ${INSTALL_DIR} - try --system or create the directory manually." >&2
  exit 1
}

TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT INT TERM

echo "Downloading ${ARCHIVE}..."
if ! curl -fsSL --progress-bar "${DOWNLOAD_URL}" -o "${TMPDIR}/${ARCHIVE}"; then
  echo "" >&2
  echo "Download failed. Check that ${VERSION} exists at:" >&2
  echo "  https://github.com/${REPO}/releases" >&2
  exit 1
fi

echo "Extracting..."
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}"

CARRIER_BIN="${TMPDIR}/carrier-${OS}-${ARCH}"
if [ ! -f "${CARRIER_BIN}" ]; then
  echo "Expected binary not found in archive: carrier-${OS}-${ARCH}" >&2
  exit 1
fi
chmod +x "${CARRIER_BIN}"

if [ "${USE_SYSTEM}" = "1" ]; then
  sudo mv "${CARRIER_BIN}" "${INSTALL_DIR}/carrier"
else
  mv "${CARRIER_BIN}" "${INSTALL_DIR}/carrier"
fi

echo ""
echo "Installed to ${INSTALL_DIR}/carrier"

if [ "${USE_SYSTEM}" != "1" ]; then
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
      echo ""
      echo "${INSTALL_DIR} is not on your PATH."
      echo "Add this to your shell profile (~/.bashrc, ~/.zshrc, or ~/.profile):"
      echo ""
      echo '  export PATH="${HOME}/.local/bin:${PATH}"'
      echo ""
      echo "Then restart your terminal or run: source ~/.profile"
      ;;
  esac
fi

echo ""
if command -v carrier >/dev/null 2>&1; then
  carrier version
else
  "${INSTALL_DIR}/carrier" version
fi
