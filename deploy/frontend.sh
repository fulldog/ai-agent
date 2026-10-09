#!/usr/bin/env bash
# Linux 前端部署。在仓库根目录执行：
#   bash deploy/frontend.sh
#
# 1. 重启容器并挂载当前目录
# 2. 进入容器内 web 目录
# 3. npm run build
set -euo pipefail

ROOT="$(pwd -P)"
CONTAINER_REF="${CONTAINER_ID:-57fee3ea99c4}"
CONTAINER_NAME="${CONTAINER_NAME:-ai-agent-frontend}"
MOUNT_DEST="${MOUNT_DEST:-/opt/www/ai-agent}"
NODE_IMAGE="${NODE_IMAGE:-node:22-bookworm}"

log() { echo "[frontend] $(date '+%F %T') $*"; }

if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi

IMAGE="${NODE_IMAGE}"
NAME="${CONTAINER_NAME}"

if docker inspect "${CONTAINER_REF}" >/dev/null 2>&1; then
  IMAGE="$(docker inspect -f '{{.Config.Image}}' "${CONTAINER_REF}")"
  old_name="$(docker inspect -f '{{.Name}}' "${CONTAINER_REF}" | sed 's#^/##')"
  if [[ -n "${old_name}" ]]; then
    NAME="${old_name}"
  fi
  # 尽量沿用原挂载目标；没有则用默认
  old_dest="$(docker inspect -f '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "${CONTAINER_REF}" | head -1 || true)"
  if [[ -n "${old_dest}" ]]; then
    MOUNT_DEST="${old_dest%/}"
  fi
  log "停止并移除旧容器 ${CONTAINER_REF}（镜像 ${IMAGE}）"
  docker stop "${CONTAINER_REF}" >/dev/null
  docker rm "${CONTAINER_REF}" >/dev/null
elif docker inspect "${NAME}" >/dev/null 2>&1; then
  IMAGE="$(docker inspect -f '{{.Config.Image}}' "${NAME}")"
  old_dest="$(docker inspect -f '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "${NAME}" | head -1 || true)"
  if [[ -n "${old_dest}" ]]; then
    MOUNT_DEST="${old_dest%/}"
  fi
  log "停止并移除旧容器 ${NAME}（镜像 ${IMAGE}）"
  docker stop "${NAME}" >/dev/null
  docker rm "${NAME}" >/dev/null
fi

log "启动容器 ${NAME}，挂载 ${ROOT} → ${MOUNT_DEST}"
docker run -d \
  --name "${NAME}" \
  -v "${ROOT}:${MOUNT_DEST}" \
  -w "${MOUNT_DEST}/web" \
  "${IMAGE}" \
  sleep infinity >/dev/null

log "编译 ${MOUNT_DEST}/web"
docker exec -w "${MOUNT_DEST}/web" "${NAME}" npm run build
log "完成（容器 ${NAME}）"
