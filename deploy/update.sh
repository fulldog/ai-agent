#!/usr/bin/env bash
# 一键更新：拉取代码、编译后端、编译前端。不重启服务。
#
#   bash deploy/update.sh
#
# 可选环境变量：
#   BIN_PATH        默认 bin/ai-agent
#   CGO_ENABLED     默认 0
#   WEB_USE_DOCKER  默认 1，用 Docker 编译前端（避开宿主机 Node/GLIBC 过旧）
#   NODE_IMAGE      默认 node:22-bookworm
#   NPM_REGISTRY    可选 npm 源，例如 https://registry.npmmirror.com
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

log() { echo "[update] $(date '+%F %T') $*"; }

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "ERROR: 未找到命令 $1"
    exit 1
  fi
}

BIN_PATH="${BIN_PATH:-bin/ai-agent}"

need git
log "git pull"
git pull

need go
mkdir -p "$(dirname "${BIN_PATH}")"
log "编译后端 → ${BIN_PATH}"
CGO_ENABLED="${CGO_ENABLED:-0}" go build -trimpath -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/server
log "后端编译完成"

build_web_host() {
  need npm
  log "宿主机编译前端"
  (
    cd web
    if [[ -f package-lock.json ]]; then
      npm ci
    else
      npm install
    fi
    npm run build
  )
}

build_web_docker() {
  need docker
  if ! docker info >/dev/null 2>&1; then
    log "ERROR: Docker 未运行或当前用户无权限"
    exit 1
  fi
  local image="${NODE_IMAGE:-node:22-bookworm}"
  log "Docker 编译前端（镜像 ${image}）"
  mkdir -p "${REPO_ROOT}/web/dist"
  local -a args=(
    run --rm
    -e npm_config_update_notifier=false
    -v "${REPO_ROOT}/web:/app:z"
    -v ai-agent-web-npm:/app/node_modules
    -w /app
  )
  if [[ -n "${NPM_REGISTRY:-}" ]]; then
    args+=(-e "npm_config_registry=${NPM_REGISTRY}")
    log "npm registry: ${NPM_REGISTRY}"
  fi
  if [[ "$(id -u)" -ne 0 ]]; then
    args+=(--user "$(id -u):$(id -g)" -e HOME=/tmp -e npm_config_cache=/tmp/npm)
  fi
  docker "${args[@]}" "${image}" bash -lc '
    set -euo pipefail
    if [[ -f package-lock.json ]]; then
      npm ci
    else
      npm install
    fi
    npm run build
  '
}

if [[ "${WEB_USE_DOCKER:-1}" == "1" ]]; then
  build_web_docker
else
  build_web_host
fi

if [[ ! -f "${REPO_ROOT}/web/dist/index.html" ]]; then
  log "ERROR: 未生成 web/dist/index.html"
  exit 1
fi

log "完成。后端 ${BIN_PATH} ，前端 web/dist 。服务未重启，需要的话请自行重启。"
