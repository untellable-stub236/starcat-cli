#!/usr/bin/env sh
# Install the latest verified Starcat CLI release on macOS or Linux.

set -eu

repository="${STARCAT_GITHUB_REPOSITORY:-starcat-app/starcat-cli}"
install_dir="${STARCAT_INSTALL_DIR:-${HOME}/.local/bin}"
version="${STARCAT_VERSION:-}"

echo "Starcat CLI installer"
echo
echo "[1/6] Checking required commands..."

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "starcat installer: required command not found: $1" >&2
    exit 1
  fi
}

require_command curl
require_command tar

echo "[2/6] Resolving release version..."
if [ -z "${version}" ]; then
  metadata="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
    "https://api.github.com/repos/${repository}/releases/latest")"
  version="$(printf '%s\n' "${metadata}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi
if [ -z "${version}" ]; then
  echo "starcat installer: could not determine the latest release version" >&2
  exit 1
fi
echo "      Release: ${version}"

echo "[3/6] Detecting platform..."
case "$(uname -s)" in
  Darwin) target_os="darwin" ;;
  Linux) target_os="linux" ;;
  *)
    echo "starcat installer: unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) target_arch="arm64" ;;
  x86_64|amd64) target_arch="amd64" ;;
  *)
    echo "starcat installer: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac
echo "      Target: ${target_os}/${target_arch}"

archive="starcat_${version}_${target_os}_${target_arch}.tar.gz"
release_base="${STARCAT_RELEASE_BASE_URL:-https://github.com/${repository}/releases/download/${version}}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT HUP INT TERM

echo "[4/6] Downloading ${archive} and checksums.txt..."
curl -fsSL "${release_base}/${archive}" -o "${temporary_dir}/${archive}"
curl -fsSL "${release_base}/checksums.txt" -o "${temporary_dir}/checksums.txt"

echo "[5/6] Verifying SHA-256 checksum..."
expected="$(awk -v file="${archive}" '$2 == file || $2 == "*" file { print $1; exit }' "${temporary_dir}/checksums.txt")"
if [ -z "${expected}" ]; then
  echo "starcat installer: checksums.txt does not contain ${archive}" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "${temporary_dir}/${archive}" | awk '{print $1}')"
else
  require_command shasum
  actual="$(shasum -a 256 "${temporary_dir}/${archive}" | awk '{print $1}')"
fi
if [ "${actual}" != "${expected}" ]; then
  echo "starcat installer: checksum verification failed for ${archive}" >&2
  exit 1
fi
echo "      Checksum: OK"

echo "[6/6] Installing Starcat CLI..."
tar -xzf "${temporary_dir}/${archive}" -C "${temporary_dir}" starcat
mkdir -p "${install_dir}"
install -m 0755 "${temporary_dir}/starcat" "${install_dir}/starcat"

echo
echo "✓ Installed Starcat CLI ${version} to ${install_dir}/starcat"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *)
    echo
    echo "! ${install_dir} is not currently in PATH."
    echo "  Run this command before using starcat:"
    echo "  export PATH=\"${install_dir}:\$PATH\""
    ;;
esac

echo
echo "Get started:"
echo "  1. Open Starcat > Settings > MCP Service."
echo "  2. Click Copy Pairing Command, paste it into this terminal, and press Enter."
echo "  3. Approve the device in Starcat."
echo "  4. Run: starcat doctor"
echo
echo "Common commands:"
echo "  starcat help"
echo "  starcat capabilities"
echo "  starcat repo search \"local RAG\""
