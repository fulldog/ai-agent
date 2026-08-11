#!/usr/bin/env bash
# 服务重启脚本。
#
# 用法:
#   bash deploy/docker/restart-app.sh [模式]
#
# 模式:
#   restart | quick | r   — 仅重启 app 容器（不拉代码、不编译）
#   rebuild | build | b   — 拉 Git → 编译镜像 → 重启（默认）
#
# 示例:
#   bash deploy/docker/restart-app.sh              # 等同 rebuild
#   bash deploy/docker/restart-app.sh restart      # 只重启
#   bash deploy/docker/restart-app.sh rebuild      # 拉代码并重新编译
#
# 环境变量（可选）:
#   DEPLOY_BRANCH / DEPLOY_REMOTE / COMPOSE_FILE / COMPOSE_DB_FILE / APP_PORT / GOPROXY / DB_MODE
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

usage() {
  cat <<'EOF'
用法: bash deploy/docker/restart-app.sh [模式]

模式:
  restart | quick | r   仅重启 app（不拉代码、不编译）
  rebuild | build | b   拉 Git → 编译镜像 → 重启（默认）

示例:
  bash deploy/docker/restart-app.sh restart
  bash deploy/docker/restart-app.sh rebuild
EOF
}

MODE_RAW="${1:-rebuild}"
case "${MODE_RAW}" in
  restart|quick|r)
    MODE="restart"
    ;;
  rebuild|build|full|b)
    MODE="rebuild"
    ;;
  -h|--help|help)
    usage
    exit 0
    ;;
  *)
    log "ERROR: 未知模式 '${MODE_RAW}'"
    usage
    exit 1
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  log "ERROR: 未找到 docker"
  exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
  log "ERROR: 需要 Docker Compose V2"
  exit 1
fi

if [[ "${MODE}" == "rebuild" ]] && ! command -v git >/dev/null 2>&1; then
  log "ERROR: 未找到 git（rebuild 模式需要）"
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

read_config_dsn() {
  grep -E '^[[:space:]]*dsn:' "${CONFIG_FILE}" | head -1 | sed -E 's/^[[:space:]]*dsn:[[:space:]]*//' | sed -E 's/^["'\'']//;s/["'\'']$//'
}

rewrite_dsn_host() {
  local dsn="$1"
  local newhost="$2"
  echo "${dsn}" | sed -E "s#(@)[^/:]+([:/])#\\1${newhost}\\2#"
}

our_db_running() {
  docker ps --format '{{.Names}}' 2>/dev/null | grep -qx 'ai-agent-db'
}

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

CFG_DSN="$(read_config_dsn)"
if [[ -z "${CFG_DSN}" ]]; then
  log "ERROR: ${CONFIG_FILE} 中未找到 database.dsn"
  exit 1
fi

DB_MODE="${DB_MODE:-auto}"
PG_PORT="${POSTGRES_PORT:-5432}"
USE_EMBEDDED=0
case "${DB_MODE}" in
  embedded|internal|always)
    USE_EMBEDDED=1
    ;;
  external|exist|existing)
    USE_EMBEDDED=0
    ;;
  auto|*)
    if our_db_running; then
      USE_EMBEDDED=1
    elif port_listening "${PG_PORT}"; then
      USE_EMBEDDED=0
    else
      USE_EMBEDDED=1
      log "WARN: 未检测到库；将按内置库主机 db 设置 DATABASE_URL（请确认 ai-agent-db 已运行）"
    fi
    ;;
esac

unset DATABASE_URL || true
unset DATABASE_ENABLED || true

compose_files=(-f "${COMPOSE_APP}")
if [[ "${USE_EMBEDDED}" -eq 1 ]]; then
  if [[ -f "${COMPOSE_DB}" ]]; then
    compose_files+=(-f "${COMPOSE_DB}")
  fi
  if [[ "${CFG_DSN}" == *"@db:"* ]]; then
    export DATABASE_URL="${CFG_DSN}"
  else
    export DATABASE_URL
    DATABASE_URL="$(rewrite_dsn_host "${CFG_DSN}" "db")"
  fi
  log "内置库模式 → DATABASE_URL 主机改为 db（不改 ${CONFIG_FILE}）"
else
  if [[ "${CFG_DSN}" == *"@host.docker.internal:"* ]]; then
    export DATABASE_URL="${CFG_DSN}"
  else
    export DATABASE_URL
    DATABASE_URL="$(rewrite_dsn_host "${CFG_DSN}" "host.docker.internal")"
  fi
  log "外部库模式 → DATABASE_URL 主机改为 host.docker.internal（不改 ${CONFIG_FILE}）"
fi

log "模式: ${MODE}"

REV="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"

if [[ "${MODE}" == "rebuild" ]]; then
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

  # ---------- 3. 用新镜像重启 ----------
  log "重启容器 ai-agent-app（force-recreate）..."
  docker compose "${compose_files[@]}" up -d --no-deps --force-recreate app
else
  # ---------- 仅重启 ----------
  log "仅重启容器 ai-agent-app（跳过 git / build）..."
  if docker ps -a --format '{{.Names}}' 2>/dev/null | grep -qx 'ai-agent-app'; then
    docker compose "${compose_files[@]}" restart app
  else
    log "容器不存在，改为 up -d ..."
    docker compose "${compose_files[@]}" up -d --no-deps app
  fi
fi

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

if [[ "${MODE}" == "rebuild" ]]; then
  log "完成（rebuild）。提交 ${REV} 已部署到 ai-agent-app"
else
  log "完成（restart）。当前提交 ${REV}"
fi
