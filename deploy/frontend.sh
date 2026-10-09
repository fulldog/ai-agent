#!/usr/bin/env bash
# Linux 前端部署。在仓库根目录执行：
#   bash deploy/frontend.sh
#
# 固定容器名，不依赖会变化的容器 ID。
# 1. 重启容器并挂载当前目录
# 2. 进入容器内 web 目录
# 3. npm run build
set -euo pipefail

ROOT="$(pwd -P)"
# 固定名称，下次编译仍用同一个；不要用会变化的容器 ID
CONTAINER_NAME="${CONTAINER_NAME:-ai-agent-frontend}"
# 仅首次迁移：若旧 ID 仍在且固定名还没有，先接管它的镜像/挂载点再删掉
LEGACY_CONTAINER_ID="${LEGACY_CONTAINER_ID:-4ccd6e616234}"
MOUNT_DEST="${MOUNT_DEST:-/opt/www/ai-agent}"
NODE_IMAGE="${NODE_IMAGE:-node:22-bookworm}"

log() { echo "[frontend] $(date '+%F %T') $*"; }

if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi

IMAGE="${NODE_IMAGE}"
REF=""

if docker inspect "${CONTAINER_NAME}" >/dev/null 2>&1; then
  REF="${CONTAINER_NAME}"
elif docker inspect "${LEGACY_CONTAINER_ID}" >/dev/null 2>&1; then
  REF="${LEGACY_CONTAINER_ID}"
  log "发现旧容器 ${LEGACY_CONTAINER_ID}，将迁移为固定名 ${CONTAINER_NAME}"
fi

if [[ -n "${REF}" ]]; then
  IMAGE="$(docker inspect -f '{{.Config.Image}}' "${REF}")"
  old_dest="$(docker inspect -f '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "${REF}" | head -1 || true)"
  if [[ -n "${old_dest}" ]]; then
    MOUNT_DEST="${old_dest%/}"
  fi
  log "停止并移除容器 ${REF}（镜像 ${IMAGE}）"
  docker stop "${REF}" >/dev/null
  docker rm "${REF}" >/dev/null
fi

# 若固定名被占用（异常残留），一并清掉
if docker inspect "${CONTAINER_NAME}" >/dev/null 2>&1; then
  docker stop "${CONTAINER_NAME}" >/dev/null || true
  docker rm "${CONTAINER_NAME}" >/dev/null || true
fi

log "启动容器 ${CONTAINER_NAME}，挂载 ${ROOT} → ${MOUNT_DEST}"
docker run -d \
  --name "${CONTAINER_NAME}" \
  -v "${ROOT}:${MOUNT_DEST}" \
  -w "${MOUNT_DEST}/web" \
  "${IMAGE}" \
  sleep infinity >/dev/null

log "编译 ${MOUNT_DEST}/web"
docker exec -w "${MOUNT_DEST}/web" "${CONTAINER_NAME}" npm run build
log "完成（容器名固定为 ${CONTAINER_NAME}，下次仍用此名）"
