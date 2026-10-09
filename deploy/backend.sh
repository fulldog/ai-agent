#!/usr/bin/env bash
# Linux ARM 后端部署。在仓库根目录执行：
#   bash deploy/backend.sh
#
# 1. 编译 ./cmd/server/main.go → ./bin/ai-agent
# 2. 注册 systemd 服务（已注册则跳过），配置 ./configs/config.yaml
# 3. 重启服务
set -euo pipefail

ROOT="$(pwd)"
cd "${ROOT}"

SERVICE_NAME="${SERVICE_NAME:-ai-agent}"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
BIN_REL="./bin/ai-agent"
CONFIG_REL="./configs/config.yaml"

log() { echo "[backend] $(date '+%F %T') $*"; }

sudo_run() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  else
    sudo "$@"
  fi
}

if ! command -v go >/dev/null 2>&1; then
  log "ERROR: 未找到 go"
  exit 1
fi
if [[ ! -f "${CONFIG_REL}" ]]; then
  log "ERROR: 缺少 ${CONFIG_REL}"
  exit 1
fi

case "$(uname -s)-$(uname -m)" in
  Linux-aarch64|Linux-arm64)
    export GOOS=linux
    export GOARCH=arm64
    unset GOARM
    ;;
  Linux-armv7l|Linux-armv6l|Linux-armhf|Linux-arm)
    export GOOS=linux
    export GOARCH=arm
    export GOARM="${GOARM:-7}"
    ;;
  *)
    log "ERROR: 需要 Linux ARM，当前是 $(uname -s) $(uname -m)"
    exit 1
    ;;
esac

mkdir -p ./bin
log "编译 ${BIN_REL} (GOOS=${GOOS} GOARCH=${GOARCH})"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BIN_REL}" ./cmd/server/main.go
log "编译完成"

BIN_ABS="${ROOT}/bin/ai-agent"
CONFIG_ABS="${ROOT}/configs/config.yaml"

if [[ -f "${UNIT_PATH}" ]] || systemctl cat "${SERVICE_NAME}.service" >/dev/null 2>&1; then
  log "服务 ${SERVICE_NAME} 已注册，跳过"
else
  log "注册 systemd 服务 ${SERVICE_NAME}"
  unit_tmp="$(mktemp)"
  cat >"${unit_tmp}" <<EOF
[Unit]
Description=AI Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${ROOT}
ExecStart=${BIN_ABS} -config ${CONFIG_ABS}
Restart=on-failure
RestartSec=3
KillSignal=SIGTERM
TimeoutStopSec=15

[Install]
WantedBy=multi-user.target
EOF
  sudo_run cp "${unit_tmp}" "${UNIT_PATH}"
  rm -f "${unit_tmp}"
  sudo_run systemctl daemon-reload
  sudo_run systemctl enable "${SERVICE_NAME}.service"
  log "已注册 ${UNIT_PATH}"
fi

log "重启 ${SERVICE_NAME}"
sudo_run systemctl restart "${SERVICE_NAME}.service"
sudo_run systemctl --no-pager --full status "${SERVICE_NAME}.service" || true
log "完成"
