#!/usr/bin/env bash
# 为 GitHub Release 构建 Starcat CLI 的跨平台原始二进制。
# 本脚本只写项目内 dist/，不创建 tag、不上传 Release，也不修改 Starcat 主项目。

set -euo pipefail

version="${1:-}"
if [[ -z "${version}" ]]; then
  echo "usage: scripts/build-all.sh <version>" >&2
  exit 2
fi

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dist_dir="${project_dir}/dist"
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
  suffix=""
  if [[ "${target_os}" == "windows" ]]; then
    suffix=".exe"
  fi
  output="${dist_dir}/starcat_${version}_${target_os}_${target_arch}${suffix}"
  GOOS="${target_os}" GOARCH="${target_arch}" CGO_ENABLED=0 \
    go build -trimpath \
      -ldflags "-s -w -X github.com/dong4j/starcat-cli/internal/mcp.Version=${version}" \
      -o "${output}" \
      "${project_dir}/cmd/starcat"
done

if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "${dist_dir}"/starcat_* > "${dist_dir}/checksums.txt"
else
  shasum -a 256 "${dist_dir}"/starcat_* > "${dist_dir}/checksums.txt"
fi

echo "Built Starcat CLI ${version} artifacts in ${dist_dir}"
