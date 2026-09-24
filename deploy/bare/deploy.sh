#!/usr/bin/env bash
# 裸机部署：编译 server / web，重启 ai-agent，安装 Nginx（IP:端口访问控制台）。
#
#   bash deploy/bare/deploy.sh
#
# 环境变量（可选）:
#   CONFIG          默认 configs/config.yaml
#   BIN_PATH        默认 bin/ai-agent
#   WEB_PORT        控制台 Nginx 监听端口，默认 8080（无域名，用 http://<IP>:8080）
#   API_HOST        反代后端，默认 127.0.0.1
#   API_PORT        后端端口，默认从 config server.addr 解析，否则 18090
#   SKIP_SERVER=1   不编译、不重启 Go
#   SKIP_WEB=1      不编译前端
#   WEB_USE_DOCKER  默认 1：用 Docker 编译前端（避开宿主机 Node/GLIBC 过旧）
#   NODE_IMAGE      前端构建镜像，默认 node:22-bookworm
#   NPM_REGISTRY    可选 npm 源，例如 https://registry.npmmirror.com
#   SKIP_NGINX=1    不写 Nginx、不 reload
#   NGINX_CONF      安装路径；空则自动选 conf.d 或 sites-available
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

LOG_TAG="[ai-agent-bare]"
log() { echo "${LOG_TAG} $(date '+%F %T') $*"; }

CONFIG="${CONFIG:-configs/config.yaml}"
BIN_PATH="${BIN_PATH:-bin/ai-agent}"
PID_FILE="${PID_FILE:-data/ai-agent.pid}"
STDOUT_LOG="${STDOUT_LOG:-data/logs/server.stdout.log}"
WEB_PORT="${WEB_PORT:-8080}"
API_HOST="${API_HOST:-127.0.0.1}"
TEMPLATE="${SCRIPT_DIR}/nginx.conf.template"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "ERROR: 未找到命令 $1"
    exit 1
  fi
}

sudo_run() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    log "ERROR: 需要 root 或 sudo 才能安装 Nginx 配置"
    exit 1
  fi
}

parse_api_port() {
  local addr="${API_PORT:-}"
  if [[ -n "${addr}" ]]; then
    echo "${addr}"
    return
  fi
  if [[ -f "${CONFIG}" ]]; then
    addr="$(grep -E '^[[:space:]]*addr:' "${CONFIG}" | head -1 | sed -E 's/^[[:space:]]*addr:[[:space:]]*//;s/^["'\'']//;s/["'\'']$//;s/[[:space:]]+#.*$//' || true)"
    addr="${addr%%$'\r'}"
    if [[ "${addr}" =~ :([0-9]+)$ ]]; then
      echo "${BASH_REMATCH[1]}"
      return
    fi
  fi
  echo "18090"
}

API_PORT="$(parse_api_port)"
API_UPSTREAM="${API_HOST}:${API_PORT}"
WEB_ROOT="${REPO_ROOT}/web/dist"

# ---------- 1. 编译 server ----------
if [[ "${SKIP_SERVER:-0}" != "1" ]]; then
  need go
  mkdir -p "$(dirname "${BIN_PATH}")"
  log "编译 server → ${BIN_PATH}"
  CGO_ENABLED="${CGO_ENABLED:-0}" go build -trimpath -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/server
  log "server 编译完成: $(wc -c <"${BIN_PATH}" | tr -d ' ') bytes"
fi

# ---------- 2. 编译 web ----------
build_web_host() {
  need npm
  log "宿主机编译 web（vue-tsc + vite build）"
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
  log "Docker 编译 web（镜像 ${image}）"
  mkdir -p "${WEB_ROOT}"

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

if [[ "${SKIP_WEB:-0}" != "1" ]]; then
  if [[ "${WEB_USE_DOCKER:-1}" == "1" ]]; then
    build_web_docker
  else
    build_web_host
  fi
  if [[ ! -f "${WEB_ROOT}/index.html" ]]; then
    log "ERROR: 未生成 ${WEB_ROOT}/index.html"
    exit 1
  fi
  log "web 产物: ${WEB_ROOT}"
fi

# ---------- 3. kill 旧进程后重启 server ----------
if [[ "${SKIP_SERVER:-0}" != "1" ]]; then
  if [[ ! -f "${CONFIG}" ]]; then
    log "ERROR: 缺少 ${CONFIG}，请先复制 configs/config.example.yaml"
    exit 1
  fi
  mkdir -p data/logs data/attachments

  stop_old() {
    local pid=""
    if [[ -f "${PID_FILE}" ]]; then
      pid="$(tr -d ' \r\n' <"${PID_FILE}" || true)"
      if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
        log "停止 PID 文件中的进程 ${pid}"
        kill "${pid}" 2>/dev/null || true
        for _ in $(seq 1 20); do
          kill -0 "${pid}" 2>/dev/null || break
          sleep 0.2
        done
        if kill -0 "${pid}" 2>/dev/null; then
          log "进程未退出，改为 SIGKILL ${pid}"
          kill -9 "${pid}" 2>/dev/null || true
        fi
      fi
      rm -f "${PID_FILE}"
    fi

    local pids=""
    if command -v ss >/dev/null 2>&1; then
      pids="$(ss -lntp 2>/dev/null | awk -v p=":${API_PORT}" '$4 ~ p"$" {print}' | grep -oE 'pid=[0-9]+' | cut -d= -f2 | sort -u || true)"
    fi
    if [[ -z "${pids}" ]] && command -v lsof >/dev/null 2>&1; then
      pids="$(lsof -t -iTCP:"${API_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
    fi
    if [[ -n "${pids}" ]]; then
      log "释放端口 ${API_PORT}，结束: ${pids}"
      # shellcheck disable=SC2086
      kill ${pids} 2>/dev/null || true
      sleep 0.5
      # shellcheck disable=SC2086
      kill -9 ${pids} 2>/dev/null || true
    fi
  }

  stop_old

  BIN_ABS="$(cd "$(dirname "${BIN_PATH}")" && pwd)/$(basename "${BIN_PATH}")"
  CFG_ABS="$(cd "$(dirname "${CONFIG}")" && pwd)/$(basename "${CONFIG}")"
  log "启动 ${BIN_ABS} -config ${CFG_ABS}"
  nohup "${BIN_ABS}" -config "${CFG_ABS}" >>"${STDOUT_LOG}" 2>&1 &
  echo $! >"${PID_FILE}"
  log "已写入 ${PID_FILE} pid=$(cat "${PID_FILE}")"

  HEALTH_URL="http://${API_HOST}:${API_PORT}/health"
  ok=0
  for i in $(seq 1 40); do
    if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      log "健康检查通过: ${HEALTH_URL}"
      ok=1
      break
    fi
    sleep 0.5
  done
  if [[ "${ok}" -ne 1 ]]; then
    log "ERROR: 后端未就绪 ${HEALTH_URL}，最近日志："
    tail -n 80 "${STDOUT_LOG}" 2>/dev/null || true
    exit 1
  fi
fi

# ---------- 4. Nginx：无域名，IP + 端口 ----------
if [[ "${SKIP_NGINX:-0}" != "1" ]]; then
  if [[ ! -f "${TEMPLATE}" ]]; then
    log "ERROR: 缺少 ${TEMPLATE}"
    exit 1
  fi
  if ! command -v nginx >/dev/null 2>&1; then
    log "ERROR: 未安装 nginx。Debian/Ubuntu: apt install nginx ；RHEL: dnf install nginx"
    exit 1
  fi

  RENDERED="$(mktemp)"
  sed \
    -e "s|__WEB_PORT__|${WEB_PORT}|g" \
    -e "s|__WEB_ROOT__|${WEB_ROOT}|g" \
    -e "s|__API_UPSTREAM__|${API_UPSTREAM}|g" \
    "${TEMPLATE}" >"${RENDERED}"

  DEST="${NGINX_CONF:-}"
  ENABLE_LINK=""
  if [[ -z "${DEST}" ]]; then
    if [[ -d /etc/nginx/sites-available ]]; then
      DEST="/etc/nginx/sites-available/ai-agent-web.conf"
      ENABLE_LINK="/etc/nginx/sites-enabled/ai-agent-web.conf"
    elif [[ -d /etc/nginx/conf.d ]]; then
      DEST="/etc/nginx/conf.d/ai-agent-web.conf"
    else
      DEST="/etc/nginx/ai-agent-web.conf"
    fi
  fi

  log "安装 Nginx 配置 → ${DEST}（listen ${WEB_PORT}，server_name _）"
  sudo_run cp "${RENDERED}" "${DEST}"
  rm -f "${RENDERED}"
  if [[ -n "${ENABLE_LINK}" ]]; then
    sudo_run ln -sfn "${DEST}" "${ENABLE_LINK}"
    # 避免 default 站点抢同一个 default_server
    if [[ -L /etc/nginx/sites-enabled/default ]]; then
      log "禁用 Nginx 默认站点，避免占用端口"
      sudo_run rm -f /etc/nginx/sites-enabled/default
    fi
  fi

  sudo_run nginx -t
  if sudo_run nginx -s reload 2>/dev/null; then
    log "nginx reload 完成"
  else
    sudo_run systemctl reload nginx 2>/dev/null || sudo_run systemctl restart nginx
    log "nginx 已 reload/restart"
  fi
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
IP="${IP:-<服务器IP>}"
log "完成。"
log "  API:  http://${API_HOST}:${API_PORT}/health"
log "  控制台（无域名）: http://${IP}:${WEB_PORT}/"
log "  浏览器打开控制台后到「连接设置」填写 X-API-Key；API Base 留空（走同域 /api 反代）"
