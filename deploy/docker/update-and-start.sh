#!/usr/bin/env bash
# 从 GitHub 拉取最新代码并编译启动。
# 数据库策略（DB_MODE，默认 auto）：
#   auto     — 宿主机 5432（或 POSTGRES_PORT）已有服务则复用，并检查 pgvector；否则启动内置 db
#   external — 强制连外部库（需 EXTERNAL_DATABASE_URL）
#   embedded — 强制启动 compose 内置 PostgreSQL+pgvector
#
#   ./deploy/docker/update-and-start.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

BRANCH="${DEPLOY_BRANCH:-main}"
REMOTE="${DEPLOY_REMOTE:-origin}"
COMPOSE_APP="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_DB="${COMPOSE_DB_FILE:-docker-compose.db.yml}"
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

# 加载 .env（忽略注释与空行；值可带引号）
set -a
# shellcheck disable=SC1091
source .env
set +a

mkdir -p data/logs data/attachments
chmod +x deploy/docker/update-and-start.sh deploy/docker/ensure-pgvector.sh 2>/dev/null || true

log "仓库目录: ${REPO_ROOT}"
log "拉取 ${REMOTE}/${BRANCH} ..."

git fetch --prune "${REMOTE}"
git checkout "${BRANCH}"
git reset --hard "${REMOTE}/${BRANCH}"

REV="$(git rev-parse --short HEAD)"
log "当前提交: ${REV} ($(git log -1 --pretty=format:'%s'))"

DB_MODE="${DB_MODE:-auto}"
PG_PORT="${POSTGRES_PORT:-5432}"

port_listening() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -lnt 2>/dev/null | grep -qE ":${port}\\s" && return 0
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "${port}" >/dev/null 2>&1 && return 0
  fi
  timeout 1 bash -c "echo >/dev/tcp/127.0.0.1/${port}" >/dev/null 2>&1 && return 0
  return 1
}

our_db_running() {
  docker ps --format '{{.Names}}' 2>/dev/null | grep -qx 'ai-agent-db'
}

# 决定是否使用内置库
USE_EMBEDDED=0
case "${DB_MODE}" in
  embedded|internal|always)
    USE_EMBEDDED=1
    log "DB_MODE=${DB_MODE} → 使用内置 PostgreSQL+pgvector"
    ;;
  external|exist|existing)
    USE_EMBEDDED=0
    log "DB_MODE=${DB_MODE} → 复用外部 PostgreSQL"
    ;;
  auto|*)
    if our_db_running; then
      USE_EMBEDDED=1
      log "检测到本项目容器 ai-agent-db 已在运行 → 继续用内置库"
    elif port_listening "${PG_PORT}"; then
      USE_EMBEDDED=0
      log "检测到 ${PG_PORT} 端口已有服务 → 复用外部 PostgreSQL（不新建库容器）"
    else
      USE_EMBEDDED=1
      log "未检测到 ${PG_PORT} 上的数据库 → 启动内置 PostgreSQL+pgvector"
    fi
    ;;
esac

APP_DSN=""
if [[ "${USE_EMBEDDED}" -eq 1 ]]; then
  # 内置：容器间用主机名 db
  APP_DSN="${DATABASE_URL:-postgres://${POSTGRES_USER:-ai_agent}:${POSTGRES_PASSWORD:-ai_agent_dev}@db:5432/${POSTGRES_DB:-ai_agent}?sslmode=disable}"
  # 若用户误把 URL 写成 127.0.0.1，纠正为 db
  if [[ "${APP_DSN}" == *"@127.0.0.1:"* ]] || [[ "${APP_DSN}" == *"@localhost:"* ]]; then
    log "WARN: 内置模式下 DATABASE_URL 含 127.0.0.1/localhost，将改用主机名 db"
    APP_DSN="postgres://${POSTGRES_USER:-ai_agent}:${POSTGRES_PASSWORD:-ai_agent_dev}@db:5432/${POSTGRES_DB:-ai_agent}?sslmode=disable"
  fi
  export DATABASE_URL="${APP_DSN}"

  log "构建并启动：app + 内置 db ..."
  docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" up -d --build --remove-orphans

  log "等待内置库就绪并确认 pgvector ..."
  # 等 health
  for i in $(seq 1 60); do
    if docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" exec -T db \
      pg_isready -U "${POSTGRES_USER:-ai_agent}" -d "${POSTGRES_DB:-ai_agent}" >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  HOST_CHECK_DSN="postgres://${POSTGRES_USER:-ai_agent}:${POSTGRES_PASSWORD:-ai_agent_dev}@127.0.0.1:${PG_PORT}/${POSTGRES_DB:-ai_agent}?sslmode=disable"
  ./deploy/docker/ensure-pgvector.sh "${HOST_CHECK_DSN}"
else
  # 外部库：应用容器经 host.docker.internal 访问宿主机映射端口
  EXT_DSN="${EXTERNAL_DATABASE_URL:-}"
  if [[ -z "${EXT_DSN}" ]]; then
    # 若 DATABASE_URL 已指向非 db 主机，也可直接用
    if [[ -n "${DATABASE_URL:-}" && "${DATABASE_URL}" != *"@db:"* ]]; then
      EXT_DSN="${DATABASE_URL}"
    fi
  fi
  if [[ -z "${EXT_DSN}" ]]; then
    log "ERROR: 将复用外部 Postgres，但未配置 EXTERNAL_DATABASE_URL。"
    log "  请在 .env 中设置，例如："
    log "  EXTERNAL_DATABASE_URL=postgres://用户:密码@127.0.0.1:5432/库名?sslmode=disable"
    log "  （脚本会自动把应用侧改成 host.docker.internal）"
    exit 1
  fi

  # 宿主机检测用
  CHECK_DSN="${EXT_DSN}"
  # 应用容器用
  APP_DSN="${EXT_DSN}"
  APP_DSN="${APP_DSN//@127.0.0.1:/@host.docker.internal:}"
  APP_DSN="${APP_DSN//@localhost:/@host.docker.internal:}"
  export DATABASE_URL="${APP_DSN}"
  export EXTERNAL_DATABASE_URL="${CHECK_DSN}"

  log "检查外部库与 pgvector ..."
  ./deploy/docker/ensure-pgvector.sh "${CHECK_DSN}"

  # 若本项目以前起过内置 db，避免占端口；仅停 db，不删卷
  if our_db_running; then
    log "停止本项目旧的 ai-agent-db 容器（保留数据卷）..."
    docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" stop db >/dev/null 2>&1 || true
  fi

  log "构建并启动：仅 app（连接外部库）..."
  # 不用 --remove-orphans，避免误删其它项目容器；显式不启动 db
  docker compose -f "${COMPOSE_APP}" up -d --build
fi

log "服务状态:"
docker compose -f "${COMPOSE_APP}" ps -a
if [[ "${USE_EMBEDDED}" -eq 1 ]]; then
  docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" ps -a
fi

HEALTH_URL="http://127.0.0.1:${APP_PORT:-18090}/health"
log "等待应用就绪: ${HEALTH_URL}"
ok=0
for i in $(seq 1 60); do
  if curl -fsS "${HEALTH_URL}" >/tmp/ai-agent-health.$$ 2>/dev/null; then
    log "健康检查通过: $(cat /tmp/ai-agent-health.$$)"
    ok=1
    break
  fi
  # 容器已退出则不必空等
  status="$(docker inspect -f '{{.State.Status}}' ai-agent-app 2>/dev/null || echo missing)"
  if [[ "${status}" == "exited" || "${status}" == "dead" || "${status}" == "missing" ]]; then
    log "ERROR: 容器 ai-agent-app 状态=${status}，启动失败"
    break
  fi
  sleep 2
done
rm -f /tmp/ai-agent-health.$$

if [[ "${ok}" -ne 1 ]]; then
  log "ERROR: 应用未在 ${HEALTH_URL} 就绪。最近日志："
  docker logs --tail 80 ai-agent-app 2>&1 || true
  log "请执行: docker ps -a | grep ai-agent; docker logs -f ai-agent-app"
  exit 1
fi

log "完成。DATABASE_URL(app)=${DATABASE_URL//:*@/:***@}"
