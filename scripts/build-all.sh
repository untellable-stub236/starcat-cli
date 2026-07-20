#!/usr/bin/env bash
# Build release archives locally. This script never creates tags or publishes releases.

set -euo pipefail

version="${1:-}"
if [[ ! "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "Usage: scripts/build-all.sh <vMAJOR.MINOR.PATCH>" >&2
  exit 2
fi

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${project_dir}/dist"
stage_root="$(mktemp -d)"
trap 'rm -rf "${stage_root}"' EXIT

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"

targets=(
  "darwin arm64"
  "darwin amd64"
  "linux arm64"
  "linux amd64"
  "windows amd64"
)

for target in "${targets[@]}"; do
  read -r target_os target_arch <<< "${target}"
  stage_dir="${stage_root}/${target_os}-${target_arch}"
  mkdir -p "${stage_dir}"
  binary_name="starcat"
  if [[ "${target_os}" == "windows" ]]; then
    binary_name="starcat.exe"
  fi

  CGO_ENABLED=0 GOOS="${target_os}" GOARCH="${target_arch}" \
    go build -trimpath \
      -ldflags "-s -w -X github.com/dong4j/starcat-cli/internal/mcp.Version=${version}" \
      -o "${stage_dir}/${binary_name}" \
      "${project_dir}/cmd/starcat"

  cp "${project_dir}/LICENSE" "${stage_dir}/LICENSE"
  cp "${project_dir}/THIRD_PARTY_NOTICES.md" "${stage_dir}/THIRD_PARTY_NOTICES.md"

  archive_base="starcat_${version}_${target_os}_${target_arch}"
  if [[ "${target_os}" == "windows" ]]; then
    (
      cd "${stage_dir}"
      zip -q -X "${dist_dir}/${archive_base}.zip" "${binary_name}" LICENSE THIRD_PARTY_NOTICES.md
    )
  else
    tar -C "${stage_dir}" -czf "${dist_dir}/${archive_base}.tar.gz" \
      "${binary_name}" LICENSE THIRD_PARTY_NOTICES.md
  fi
done

cp "${project_dir}/scripts/install.sh" "${dist_dir}/install.sh"
cp "${project_dir}/scripts/install.ps1" "${dist_dir}/install.ps1"
chmod 0755 "${dist_dir}/install.sh"

(
  cd "${dist_dir}"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum starcat_* install.sh install.ps1 > checksums.txt
  else
    shasum -a 256 starcat_* install.sh install.ps1 > checksums.txt
  fi
)

echo "Built Starcat CLI ${version} release assets in ${dist_dir}"
