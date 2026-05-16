#!/bin/sh
# install.sh — download and install the skillpack binary
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh \
#     -o /tmp/skillpack-install.sh && sh /tmp/skillpack-install.sh
#
# Override install directory (env var must be passed to sh, not curl):
#   curl -fsSL https://raw.githubusercontent.com/bmaltais/skillpack/main/install.sh \
#     -o /tmp/skillpack-install.sh \
#     && sudo env SKILLPACK_INSTALL_DIR=/usr/local/bin sh /tmp/skillpack-install.sh
#
# Supported platforms: Linux (amd64, arm64), macOS (amd64, arm64), Windows (amd64, arm64)
# Windows: use install.bat instead of this script.

set -e

REPO="bmaltais/skillpack"
BINARY="skillpack"
BASE_URL="https://github.com/${REPO}/releases/latest/download"

# ── Detect OS ──────────────────────────────────────────────────────────────────
OS="$(uname -s)"
case "${OS}" in
  Linux)   os="linux"  ;;
  Darwin)  os="darwin" ;;
  *)
    echo "error: unsupported operating system: ${OS}" >&2
    echo "       This script supports Linux and macOS only." >&2
    echo "       For Windows, run:" >&2
    echo "       curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.bat -o skillpack-install.bat && skillpack-install.bat" >&2
    exit 1
    ;;
esac

# ── Detect arch ────────────────────────────────────────────────────────────────
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64)          arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *)
    echo "error: unsupported architecture: ${ARCH}" >&2
    echo "       Supported: x86_64, aarch64/arm64" >&2
    exit 1
    ;;
esac

# ── Choose install directory ───────────────────────────────────────────────────
if [ -n "${SKILLPACK_INSTALL_DIR}" ]; then
  install_dir="${SKILLPACK_INSTALL_DIR}"
elif { [ -d "${HOME}/.local/bin" ] || mkdir -p "${HOME}/.local/bin" 2>/dev/null; } \
      && [ -w "${HOME}/.local/bin" ]; then
  install_dir="${HOME}/.local/bin"
elif { [ -d "${HOME}/bin" ] || mkdir -p "${HOME}/bin" 2>/dev/null; } \
      && [ -w "${HOME}/bin" ]; then
  install_dir="${HOME}/bin"
else
  echo "error: could not find or create a writable install directory." >&2
  echo "       Set SKILLPACK_INSTALL_DIR to an existing writable path." >&2
  exit 1
fi

# Create the directory if it does not already exist
mkdir -p "${install_dir}"

# ── Build asset URL ────────────────────────────────────────────────────────────
asset="${BINARY}-${os}-${arch}"
url="${BASE_URL}/${asset}"
dest="${install_dir}/${BINARY}"

# ── Download (to a temp file, then atomically replace) ────────────────────────
tmp_dest="${install_dir}/.${BINARY}-tmp.$$"
trap 'rm -f "${tmp_dest}"' EXIT INT TERM

echo "Downloading ${asset} → ${dest}"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "${url}" -o "${tmp_dest}"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmp_dest}" "${url}"
else
  echo "error: neither curl nor wget is available." >&2
  exit 1
fi

chmod +x "${tmp_dest}"
mv "${tmp_dest}" "${dest}"

echo ""
echo "skillpack installed to ${dest}"

# ── PATH hint ──────────────────────────────────────────────────────────────────
case ":${PATH}:" in
  *":${install_dir}:"*)
    # already on PATH — nothing to do
    ;;
  *)
    echo ""
    echo "NOTE: ${install_dir} is not on your PATH."
    echo "      Add it by running one of the following, then restart your shell:"
    echo ""
    echo "      # bash"
    echo "      echo 'export PATH=\"${install_dir}:\$PATH\"' >> ~/.bashrc"
    echo ""
    echo "      # zsh"
    echo "      echo 'export PATH=\"${install_dir}:\$PATH\"' >> ~/.zshrc"
    echo ""
    echo "      # fish"
    echo "      fish_add_path \"${install_dir}\""
    ;;
esac
