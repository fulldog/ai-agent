#!/usr/bin/env bash
# 检查 Postgres；若未安装 pgvector 扩展文件，则尝试装进「占用本机端口」的 Docker 容器，再 CREATE EXTENSION。
# 用法：
#   ./deploy/docker/ensure-pgvector.sh "postgres://user:pass@127.0.0.1:5432/dbname?sslmode=disable"
#
# 环境变量（可选）：
#   PG_CLIENT_IMAGE   探测用客户端镜像，默认 postgres:16
#   PGVECTOR_REF      源码版本/分支，默认 v0.8.0
#   PGVECTOR_SKIP_INSTALL=1  仅检查、不自动编译安装
set -euo pipefail

LOG_TAG="[ai-agent-pgvector]"
log() { echo "${LOG_TAG} $(date '+%F %T') $*"; }

DSN="${1:-}"
if [[ -z "${DSN}" ]]; then
  log "ERROR: 缺少连接串参数"
  exit 1
fi

DSN_HOST="${DSN//host.docker.internal/127.0.0.1}"
DSN_HOST="${DSN_HOST//@db:/@127.0.0.1:}"

CLIENT_IMAGE="${PG_CLIENT_IMAGE:-postgres:16}"
PGVECTOR_REF="${PGVECTOR_REF:-v0.8.0}"
WORK_DIR="${TMPDIR:-/tmp}/ai-agent-pgvector-build"
SRC_DIR="${WORK_DIR}/pgvector"

# 从 DSN 解析端口（默认 5432）
parse_port() {
  local u="$1"
  if [[ "$u" =~ @[^:/]+:([0-9]+) ]]; then
    echo "${BASH_REMATCH[1]}"
  else
    echo "5432"
  fi
}

# 是否本机地址（才能往本机 Docker 里装扩展）
is_local_host() {
  local u="$1"
  [[ "$u" =~ @127\.0\.0\.1: ]] || [[ "$u" =~ @localhost: ]] || [[ "$u" =~ @\[::1\]: ]]
}

PG_PORT="$(parse_port "${DSN_HOST}")"

log "使用客户端镜像: ${CLIENT_IMAGE}"
log "目标端口: ${PG_PORT}"

run_psql() {
  docker run --rm --network host "${CLIENT_IMAGE}" psql "${DSN_HOST}" -v ON_ERROR_STOP=1 "$@"
}

if ! run_psql -At -c "SELECT version();" >/tmp/ai-agent-pg-ver.$$ 2>/tmp/ai-agent-pg-err.$$; then
  log "ERROR: 无法连接数据库。请检查连接串 / 账号密码 / 库是否已创建。"
  sed 's/^/  /' /tmp/ai-agent-pg-err.$$ || true
  rm -f /tmp/ai-agent-pg-ver.$$ /tmp/ai-agent-pg-err.$$
  exit 1
fi
PG_VER="$(cat /tmp/ai-agent-pg-ver.$$)"
rm -f /tmp/ai-agent-pg-ver.$$ /tmp/ai-agent-pg-err.$$
log "PostgreSQL: ${PG_VER}"

AVAILABLE_VER="$(run_psql -At -c "SELECT COALESCE(default_version,'') FROM pg_available_extensions WHERE name='vector';" || true)"
INSTALLED_VER="$(run_psql -At -c "SELECT COALESCE(extversion,'') FROM pg_extension WHERE extname='vector';" || true)"

if [[ -n "${AVAILABLE_VER}" ]]; then
  log "pgvector 扩展文件已存在，可用版本: ${AVAILABLE_VER}；当前库已启用: ${INSTALLED_VER:-"(尚未 CREATE EXTENSION)"}"
  run_psql -c "CREATE EXTENSION IF NOT EXISTS vector;"
  EXT_VER="$(run_psql -At -c "SELECT extversion FROM pg_extension WHERE extname='vector';")"
  log "pgvector 已启用，版本: ${EXT_VER}"
  exit 0
fi

log "当前实例未安装 pgvector 扩展文件（pg_available_extensions 无 vector）"

if [[ "${PGVECTOR_SKIP_INSTALL:-0}" == "1" ]]; then
  log "ERROR: 已设置 PGVECTOR_SKIP_INSTALL=1，跳过自动安装。"
  exit 1
fi

if ! is_local_host "${DSN_HOST}"; then
  log "ERROR: 连接串不是本机地址，无法自动向远端机器安装扩展。"
  log "       请在数据库所在主机安装 pgvector，或把库换成 pgvector/pgvector 镜像。"
  exit 1
fi

# ---------- 查找占用端口的 Postgres 容器 ----------
find_container() {
  local port="$1"
  local line id
  while IFS= read -r line; do
    # 匹配 0.0.0.0:5432->5432/tcp 或 :::5432->5432/tcp 或 127.0.0.1:5432->...
    if echo "${line}" | grep -qE "(:|::)${port}->[0-9]+/tcp"; then
      id="$(echo "${line}" | awk '{print $1}')"
      echo "${id}"
      return 0
    fi
  done < <(docker ps --format '{{.ID}} {{.Ports}}')
  return 1
}

CID="$(find_container "${PG_PORT}" || true)"
if [[ -z "${CID}" ]]; then
  log "ERROR: 未找到映射了 ${PG_PORT} 端口的 Docker 容器，无法自动编译安装。"
  log "       若 Postgres 装在宿主机而非容器：请按 https://github.com/pgvector/pgvector 手动安装。"
  log "       若在容器但未映射到宿主机端口：请指定该容器并手工安装。"
  exit 1
fi

CNAME="$(docker inspect -f '{{.Name}}' "${CID}" | sed 's#^/##')"
log "将在容器内安装 pgvector: ${CNAME} (${CID:0:12})"

# ---------- 宿主机拉取源码（便于走代理 / 国内网络）----------
mkdir -p "${WORK_DIR}"
if [[ ! -d "${SRC_DIR}/.git" ]]; then
  rm -rf "${SRC_DIR}"
  log "克隆 pgvector ${PGVECTOR_REF} ..."
  if ! git clone --depth 1 --branch "${PGVECTOR_REF}" https://github.com/pgvector/pgvector.git "${SRC_DIR}"; then
    log "WARN: 指定 tag 克隆失败，改为默认分支浅克隆"
    rm -rf "${SRC_DIR}"
    git clone --depth 1 https://github.com/pgvector/pgvector.git "${SRC_DIR}"
  fi
else
  log "复用已有源码目录: ${SRC_DIR}"
fi

log "复制源码到容器 /tmp/pgvector ..."
docker exec -u root "${CID}" rm -rf /tmp/pgvector
docker cp "${SRC_DIR}" "${CID}:/tmp/pgvector"

# ---------- 容器内安装编译依赖并 make install ----------
log "在容器内编译安装（需能 apt/apk，且可写 Postgres lib 目录）..."
docker exec -u root -e DEBIAN_FRONTEND=noninteractive "${CID}" bash -lc '
set -euo pipefail
cd /tmp/pgvector

if command -v pg_config >/dev/null 2>&1; then
  PG_CONFIG="$(command -v pg_config)"
elif [[ -x /usr/lib/postgresql/*/bin/pg_config ]]; then
  PG_CONFIG="$(ls -1 /usr/lib/postgresql/*/bin/pg_config | tail -1)"
elif [[ -x /usr/pgsql-*/bin/pg_config ]]; then
  PG_CONFIG="$(ls -1 /usr/pgsql-*/bin/pg_config | tail -1)"
else
  echo "ERROR: 容器内找不到 pg_config"
  exit 1
fi
echo "pg_config=$PG_CONFIG"
echo "version=$($PG_CONFIG --version)"
PG_MAJOR="$($PG_CONFIG --version | grep -oE "[0-9]+" | head -1)"
export PG_CONFIG

install_deps_apt() {
  apt-get update -y
  apt-get install -y --no-install-recommends \
    build-essential clang llvm git ca-certificates \
    "postgresql-server-dev-${PG_MAJOR}" || \
  apt-get install -y --no-install-recommends \
    build-essential clang llvm git ca-certificates postgresql-server-dev-all
}

install_deps_apk() {
  # 官方 postgres alpine 变体
  apk add --no-cache build-base clang19 llvm19-dev git \
    "postgresql${PG_MAJOR}-dev" || \
  apk add --no-cache build-base git clang llvm-dev postgresql-dev
}

if command -v apt-get >/dev/null 2>&1; then
  install_deps_apt
elif command -v apk >/dev/null 2>&1; then
  install_deps_apk
else
  echo "ERROR: 不支持的容器系统（无 apt-get/apk），请手动安装 pgvector 或改用 pgvector/pgvector 镜像"
  exit 1
fi

make clean || true
make OPTFLAGS=""
make install
echo "pgvector files installed OK"
'

log "重新检查扩展并 CREATE EXTENSION ..."
AVAILABLE_VER="$(run_psql -At -c "SELECT COALESCE(default_version,'') FROM pg_available_extensions WHERE name='vector';" || true)"
if [[ -z "${AVAILABLE_VER}" ]]; then
  log "ERROR: 编译安装后仍检测不到 vector。请查看上方容器内编译日志，或改用镜像 pgvector/pgvector:pg${PG_MAJOR:-16}"
  exit 1
fi

run_psql -c "CREATE EXTENSION IF NOT EXISTS vector;"
EXT_VER="$(run_psql -At -c "SELECT extversion FROM pg_extension WHERE extname='vector';")"
log "成功：pgvector 已安装并启用，版本: ${EXT_VER}"
