#!/usr/bin/env sh
# Install the latest verified Starcat CLI release on macOS or Linux.

set -eu

repository="${STARCAT_GITHUB_REPOSITORY:-dong4j/starcat-cli}"
install_dir="${STARCAT_INSTALL_DIR:-${HOME}/.local/bin}"
version="${STARCAT_VERSION:-}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "starcat installer: required command not found: $1" >&2
    exit 1
  fi
}

require_command curl
require_command tar

if [ -z "${version}" ]; then
  metadata="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
    "https://api.github.com/repos/${repository}/releases/latest")"
  version="$(printf '%s\n' "${metadata}" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi
if [ -z "${version}" ]; then
  echo "starcat installer: could not determine the latest release version" >&2
  exit 1
fi

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

archive="starcat_${version}_${target_os}_${target_arch}.tar.gz"
release_base="${STARCAT_RELEASE_BASE_URL:-https://github.com/${repository}/releases/download/${version}}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT HUP INT TERM

curl -fsSL "${release_base}/${archive}" -o "${temporary_dir}/${archive}"
curl -fsSL "${release_base}/checksums.txt" -o "${temporary_dir}/checksums.txt"

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

tar -xzf "${temporary_dir}/${archive}" -C "${temporary_dir}" starcat
mkdir -p "${install_dir}"
install -m 0755 "${temporary_dir}/starcat" "${install_dir}/starcat"

echo "Installed Starcat CLI ${version} to ${install_dir}/starcat"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to PATH before running starcat." ;;
esac
