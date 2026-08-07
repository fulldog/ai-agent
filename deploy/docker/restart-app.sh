#!/usr/bin/env bash
# 服务重启：拉最新代码 → 重新编译镜像 → 用新镜像重启 app（不动 db）。
#
#   bash deploy/docker/restart-app.sh
#
# 环境变量（可选）：
#   DEPLOY_BRANCH / DEPLOY_REMOTE / COMPOSE_FILE / COMPOSE_DB_FILE / APP_PORT / GOPROXY
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

BRANCH="${DEPLOY_BRANCH:-main}"
REMOTE="${DEPLOY_REMOTE:-origin}"
COMPOSE_APP="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_DB="${COMPOSE_DB_FILE:-docker-compose.db.yml}"
LOG_TAG="[ai-agent-restart]"

log() { echo "${LOG_TAG} $(date '+%F %T') $*"; }

if ! command -v git >/dev/null 2>&1; then
  log "ERROR: 未找到 git"
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  log "ERROR: 需要 Docker Compose V2"
  exit 1
fi

if [[ -f .env ]]; then
  ENV_TMP="$(mktemp)"
  sed 's/\r$//' .env >"${ENV_TMP}"
  set -a
  # shellcheck disable=SC1090
  source "${ENV_TMP}"
  set +a
  rm -f "${ENV_TMP}"
fi

CONFIG_FILE="configs/config.yaml"
if [[ ! -f "${CONFIG_FILE}" ]]; then
  log "ERROR: 缺少 ${CONFIG_FILE}，请先: cp configs/config.docker.yaml configs/config.yaml"
  exit 1
fi

mkdir -p data/logs data/attachments

# 内置库在跑时叠加 db compose，避免 orphan 告警（仍 --no-deps 不碰 db）
compose_files=(-f "${COMPOSE_APP}")
if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx 'ai-agent-db'; then
  if [[ -f "${COMPOSE_DB}" ]]; then
    compose_files+=(-f "${COMPOSE_DB}")
    log "检测到 ai-agent-db，compose 叠加 ${COMPOSE_DB}"
  fi
fi

# ---------- 1. 更新 Git ----------
log "仓库目录: ${REPO_ROOT}"
log "拉取 ${REMOTE}/${BRANCH} ..."
git fetch --prune "${REMOTE}"
git checkout "${BRANCH}"
git reset --hard "${REMOTE}/${BRANCH}"
REV="$(git rev-parse --short HEAD)"
log "当前提交: ${REV} ($(git log -1 --pretty=format:'%s'))"

# ---------- 2. 编译 ----------
log "编译镜像 ai-agent:local ..."
docker compose "${compose_files[@]}" build app

# ---------- 3. 用新镜像启动 / 重启 app ----------
log "重启容器 ai-agent-app ..."
docker compose "${compose_files[@]}" up -d --no-deps --force-recreate app

log "服务状态:"
docker compose "${compose_files[@]}" ps app

HEALTH_URL="http://127.0.0.1:${APP_PORT:-18090}/health"
log "等待应用就绪: ${HEALTH_URL}"
ok=0
for i in $(seq 1 60); do
  if curl -fsS "${HEALTH_URL}" >/tmp/ai-agent-health.$$ 2>/dev/null; then
    log "健康检查通过: $(cat /tmp/ai-agent-health.$$)"
    ok=1
    break
  fi
  status="$(docker inspect -f '{{.State.Status}}' ai-agent-app 2>/dev/null || echo missing)"
  if [[ "${status}" == "exited" || "${status}" == "dead" || "${status}" == "missing" ]]; then
    log "ERROR: 容器 ai-agent-app 状态=${status}"
    break
  fi
  sleep 2
done
rm -f /tmp/ai-agent-health.$$

if [[ "${ok}" -ne 1 ]]; then
  log "ERROR: 应用未就绪。最近日志："
  docker logs --tail 80 ai-agent-app 2>&1 || true
  exit 1
fi

log "完成。提交 ${REV} 已部署到 ai-agent-app"
