#!/usr/bin/env bash
# 一键更新：拉取代码，再执行后端和前端部署脚本。
#
#   bash deploy/update.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

log() { echo "[update] $(date '+%F %T') $*"; }

if ! command -v git >/dev/null 2>&1; then
  log "ERROR: 未找到 git"
  exit 1
fi

log "git pull"
git pull

log "执行 deploy/backend.sh"
bash deploy/backend.sh

log "执行 deploy/frontend.sh"
bash deploy/frontend.sh

log "完成"
