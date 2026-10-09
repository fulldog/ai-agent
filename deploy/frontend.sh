#!/usr/bin/env bash
# Linux 前端部署。在仓库根目录执行：
#   bash deploy/frontend.sh
#
# 进入已有容器编译，不删除、不重建（容器 ID 保持不变）。
# 1. 找到容器（固定名，或当前这台 4ccd6e616234）
# 2. 若已停止则启动
# 3. 在挂载目录的 web 下执行 npm run build
set -euo pipefail

ROOT="$(pwd -P)"
CONTAINER_NAME="${CONTAINER_NAME:-ai-agent-frontend}"
CONTAINER_ID="${CONTAINER_ID:-4ccd6e616234}"

log() { echo "[frontend] $(date '+%F %T') $*"; }

if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi

REF=""
if docker inspect "${CONTAINER_NAME}" >/dev/null 2>&1; then
  REF="${CONTAINER_NAME}"
elif docker inspect "${CONTAINER_ID}" >/dev/null 2>&1; then
  REF="${CONTAINER_ID}"
else
  log "ERROR: 未找到容器 ${CONTAINER_NAME} 或 ${CONTAINER_ID}"
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
done < <(docker inspect -f '{{range .Mounts}}{{.Source}}|{{.Destination}}{{"\n"}}{{end}}' "${REF}")

if [[ -z "${dest}" ]]; then
  log "ERROR: 容器 ${REF} 未挂载当前目录 ${ROOT}"
  exit 1
fi

state="$(docker inspect -f '{{.State.Status}}' "${REF}")"
if [[ "${state}" != "running" ]]; then
  log "容器 ${REF} 状态为 ${state}，启动后编译"
  docker start "${REF}" >/dev/null
else
  log "进入运行中的容器 ${REF}"
fi

log "编译 ${dest}/web"
docker exec -w "${dest}/web" "${REF}" npm run build
log "完成"
