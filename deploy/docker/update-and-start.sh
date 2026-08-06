#!/usr/bin/env bash
# 从 GitHub 拉取最新代码并完整编译、启动（应用 + PostgreSQL/pgvector）
# 用法（在仓库根目录或任意目录均可，脚本会自行定位仓库）：
#   ./deploy/docker/update-and-start.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

BRANCH="${DEPLOY_BRANCH:-main}"
REMOTE="${DEPLOY_REMOTE:-origin}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
LOG_TAG="[ai-agent-deploy]"

log() { echo "${LOG_TAG} $(date '+%F %T') $*"; }

if ! command -v git >/dev/null 2>&1; then
  log "ERROR: 未找到 git，请先安装 git"
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  log "ERROR: 需要 Docker Compose V2（docker compose）"
  exit 1
fi

if [[ ! -f .env ]]; then
  log "ERROR: 缺少 .env。请先执行："
  log "  cp deploy/docker/env.example .env && vi .env"
  exit 1
fi

mkdir -p data/logs data/attachments

log "仓库目录: ${REPO_ROOT}"
log "拉取 ${REMOTE}/${BRANCH} ..."

# 丢弃对已跟踪文件的本地改动，保留未跟踪的 .env / data/
git fetch --prune "${REMOTE}"
git checkout "${BRANCH}"
git reset --hard "${REMOTE}/${BRANCH}"

REV="$(git rev-parse --short HEAD)"
log "当前提交: ${REV} ($(git log -1 --pretty=format:'%s'))"

log "构建并启动完整栈（${COMPOSE_FILE}）..."
docker compose -f "${COMPOSE_FILE}" up -d --build --remove-orphans

log "服务状态:"
docker compose -f "${COMPOSE_FILE}" ps

log "完成。健康检查: curl -sS http://127.0.0.1:${APP_PORT:-18090}/health"
