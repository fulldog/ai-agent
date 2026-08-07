#!/usr/bin/env bash
# 从 GitHub 拉取最新代码并编译启动。
# 应用配置：宿主机 configs/config.yaml（挂载进容器），密钥/DSN 写在该文件即可。
#
# 数据库策略（可选 .env 里 DB_MODE，默认 auto）：
#   auto     — 5432 已有服务则复用并装/启 pgvector；否则启动内置 db
#   external — 强制复用外部库
#   embedded — 强制内置库
#
#   bash deploy/docker/update-and-start.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

BRANCH="${DEPLOY_BRANCH:-main}"
REMOTE="${DEPLOY_REMOTE:-origin}"
COMPOSE_APP="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_DB="${COMPOSE_DB_FILE:-docker-compose.db.yml}"
CONFIG_FILE="configs/config.yaml"
CONFIG_TEMPLATE="configs/config.docker.yaml"
LOG_TAG="[ai-agent-deploy]"

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

# 可选 .env：只用于 DB_MODE / 端口 / 内置库账号等，密钥请写 configs/config.yaml
if [[ -f .env ]]; then
  ENV_TMP="$(mktemp)"
  sed 's/\r$//' .env >"${ENV_TMP}"
  set -a
  # shellcheck disable=SC1090
  source "${ENV_TMP}"
  set +a
  rm -f "${ENV_TMP}"
fi

mkdir -p data/logs data/attachments configs

log "仓库目录: ${REPO_ROOT}"
log "拉取 ${REMOTE}/${BRANCH} ..."
git fetch --prune "${REMOTE}"
git checkout "${BRANCH}"
git reset --hard "${REMOTE}/${BRANCH}"
# 勿对本仓库脚本 chmod +x：会改 filemode（644→755），导致 git status 显示「已修改」
# 调用统一用 bash xxx.sh，不依赖可执行位

# config.yaml 被 gitignore，pull 不会覆盖；首次从模板生成
if [[ ! -f "${CONFIG_FILE}" ]]; then
  cp "${CONFIG_TEMPLATE}" "${CONFIG_FILE}"
  log "已生成 ${CONFIG_FILE}（来自 ${CONFIG_TEMPLATE}），请编辑填入 api_key / dsn 后重新执行本脚本"
  log "  vi ${CONFIG_FILE}"
  exit 1
fi

REV="$(git rev-parse --short HEAD)"
log "当前提交: ${REV} ($(git log -1 --pretty=format:'%s'))"
log "使用配置文件: ${CONFIG_FILE}（挂载到容器 /app/configs/config.yaml）"

# 从 config.yaml 读取 dsn（简单解析）
read_config_dsn() {
  grep -E '^[[:space:]]*dsn:' "${CONFIG_FILE}" | head -1 | sed -E 's/^[[:space:]]*dsn:[[:space:]]*//' | sed -E 's/^["'\'']//;s/["'\'']$//'
}

config_dsn_to_host() {
  local d="$1"
  d="${d//host.docker.internal/127.0.0.1}"
  d="${d//@db:/@127.0.0.1:}"
  echo "${d}"
}

# 把 config 里的 dsn 主机改成指定主机（保留用户密码库名端口）
rewrite_dsn_host() {
  local dsn="$1"
  local newhost="$2"
  # postgres://user:pass@HOST:port/db?...
  echo "${dsn}" | sed -E "s#(@)[^/:]+([:/])#\\1${newhost}\\2#"
}

DB_MODE="${DB_MODE:-auto}"
PG_PORT="${POSTGRES_PORT:-5432}"
CFG_DSN="$(read_config_dsn)"
if [[ -z "${CFG_DSN}" ]]; then
  log "ERROR: ${CONFIG_FILE} 中未找到 database.dsn"
  exit 1
fi

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

USE_EMBEDDED=0
case "${DB_MODE}" in
  embedded|internal|always)
    USE_EMBEDDED=1
    log "DB_MODE=${DB_MODE} → 内置 PostgreSQL+pgvector"
    ;;
  external|exist|existing)
    USE_EMBEDDED=0
    log "DB_MODE=${DB_MODE} → 复用外部 PostgreSQL"
    ;;
  auto|*)
    if our_db_running; then
      USE_EMBEDDED=1
      log "检测到 ai-agent-db → 内置库"
    elif port_listening "${PG_PORT}"; then
      USE_EMBEDDED=0
      log "检测到 ${PG_PORT} 已有服务 → 复用外部库"
    else
      USE_EMBEDDED=1
      log "未检测到 ${PG_PORT} → 启动内置库"
    fi
    ;;
esac

# 清掉可能残留的空 DATABASE_URL，避免干扰
unset DATABASE_URL || true
unset DATABASE_ENABLED || true

if [[ "${USE_EMBEDDED}" -eq 1 ]]; then
  # 容器内连 db 服务；不强制改用户文件，仅运行时覆盖
  export DATABASE_URL
  DATABASE_URL="$(rewrite_dsn_host "${CFG_DSN}" "db")"
  # 若原 dsn 主机已是 db，保持
  if [[ "${CFG_DSN}" == *"@db:"* ]]; then
    DATABASE_URL="${CFG_DSN}"
  fi
  export POSTGRES_USER="${POSTGRES_USER:-ai_agent}"
  export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-ai_agent_dev}"
  export POSTGRES_DB="${POSTGRES_DB:-ai_agent}"

  log "构建并启动 app + 内置 db ..."
  docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" up -d --build --remove-orphans

  for i in $(seq 1 60); do
    if docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" exec -T db \
      pg_isready -U "${POSTGRES_USER}" -d "${POSTGRES_DB}" >/dev/null 2>&1; then
      break
    fi
    sleep 2
  done
  bash deploy/docker/ensure-pgvector.sh "$(config_dsn_to_host "${DATABASE_URL}")"
else
  # 外部库：应用用 host.docker.internal；探测用 127.0.0.1
  APP_DSN="$(rewrite_dsn_host "${CFG_DSN}" "host.docker.internal")"
  if [[ "${CFG_DSN}" == *"@host.docker.internal:"* ]]; then
    APP_DSN="${CFG_DSN}"
  fi
  export DATABASE_URL="${APP_DSN}"
  CHECK_DSN="$(config_dsn_to_host "${APP_DSN}")"

  log "检查外部库与 pgvector（配置 dsn → ${CHECK_DSN//:*@/:***@}）..."
  bash deploy/docker/ensure-pgvector.sh "${CHECK_DSN}"

  if our_db_running; then
    log "停止旧的 ai-agent-db ..."
    docker compose -f "${COMPOSE_APP}" -f "${COMPOSE_DB}" stop db >/dev/null 2>&1 || true
  fi

  log "构建并启动 app（挂载 ${CONFIG_FILE}）..."
  docker compose -f "${COMPOSE_APP}" up -d --build
fi

log "服务状态:"
docker compose -f "${COMPOSE_APP}" ps -a

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

log "完成。改密钥/模型请编辑宿主机 ${CONFIG_FILE} 后执行: docker compose restart app"
