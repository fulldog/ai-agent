#!/usr/bin/env bash
# Linux 前端部署。在仓库根目录执行（容器已挂载该目录）：
#   bash deploy/frontend.sh
#
# 1. 进入容器 57fee3ea99c4
# 2. 切换到挂载目录下的 web
# 3. npm run build
set -euo pipefail

ROOT="$(pwd -P)"
CONTAINER_ID="${CONTAINER_ID:-57fee3ea99c4}"

log() { echo "[frontend] $(date '+%F %T') $*"; }

if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi

if ! docker inspect "${CONTAINER_ID}" >/dev/null 2>&1; then
  log "ERROR: 容器不存在: ${CONTAINER_ID}"
  exit 1
fi

dest=""
while IFS='|' read -r src dst; do
  [[ -z "${src}" || -z "${dst}" ]] && continue
  src_real="$(readlink -f "${src}" 2>/dev/null || echo "${src}")"
  if [[ "${src}" == "${ROOT}" || "${src_real}" == "${ROOT}" ]]; then
    dest="${dst%/}"
    break
  fi
done < <(docker inspect -f '{{range .Mounts}}{{.Source}}|{{.Destination}}{{"\n"}}{{end}}' "${CONTAINER_ID}")

if [[ -z "${dest}" ]]; then
  log "ERROR: 容器 ${CONTAINER_ID} 未挂载当前目录 ${ROOT}"
  exit 1
fi

log "容器 ${CONTAINER_ID} 挂载 ${ROOT} → ${dest}"
log "编译 ${dest}/web"
docker exec -w "${dest}/web" "${CONTAINER_ID}" npm run build
log "完成"
