# Docker 完整部署（Git 拉取最新代码 + 编译）

> 仓库（HTTPS）：`https://github.com/fulldog/ai-agent.git`（默认分支 `main`）  
> 前提：服务器已安装 **git**、**Docker**、**Docker Compose V2**  
> 行为：**每次启动/重启服务时** → `git pull`（reset 到远端）→ `docker compose up -d --build`（重新编译镜像并启动完整栈）

完整栈包含：

| 容器 | 说明 |
|------|------|
| `ai-agent-app` | Go 应用（镜像内含 Poppler + Tesseract 中文 OCR） |
| `ai-agent-db` | PostgreSQL **16** + **pgvector**（与宿主机旧版 PG 无关） |

本地机密（`.env`）与数据（`data/`、Postgres volume）**不进 Git**，拉取代码不会覆盖。

---

## 1. 服务器一次性准备

### 1.1 安装依赖（以 RHEL / CentOS / Rocky 为例）

```bash
# git（你方环境默认已有则可跳过）
sudo yum install -y git

# Docker + Compose 插件（按发行版文档安装；示意）
sudo yum install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo systemctl enable --now docker
docker version
docker compose version
```

Debian/Ubuntu 用 `apt` 安装 `git`、`docker.io` 或官方 Docker CE，并确认有 `docker compose` 子命令。

### 1.2 用 HTTPS 克隆（推荐）

远程地址：

```text
https://github.com/fulldog/ai-agent.git
```

**公开仓库**直接克隆：

```bash
sudo mkdir -p /opt
sudo git clone https://github.com/fulldog/ai-agent.git /opt/ai-agent
sudo chown -R "$USER":"$USER" /opt/ai-agent
cd /opt/ai-agent
git checkout main
git remote -v
# origin  https://github.com/fulldog/ai-agent.git (fetch/push)
```

**私有仓库**用 GitHub Personal Access Token（PAT，至少 `repo` 只读权限）：

```bash
# 方式 A：克隆时带 Token（Token 会进 remote URL，注意权限）
sudo git clone "https://<GITHUB_USERNAME>:<GITHUB_PAT>@github.com/fulldog/ai-agent.git" /opt/ai-agent
sudo chown -R "$USER":"$USER" /opt/ai-agent
cd /opt/ai-agent

# 建议立刻改成不带密码的 URL，改用凭证存储（见下）
git remote set-url origin https://github.com/fulldog/ai-agent.git
```

长期拉取（systemd 重启会 `git fetch`）推荐把凭证存本地，避免写进 remote URL：

```bash
cd /opt/ai-agent
git remote set-url origin https://github.com/fulldog/ai-agent.git

# 按提示输入用户名；密码处粘贴 PAT（不是 GitHub 登录密码）
git config --global credential.helper store
git fetch origin
# 首次成功后，凭证写入 ~/.git-credentials

chmod 600 ~/.git-credentials
```

或只给本仓库写死（机器专用账号时可用；注意文件权限）：

```bash
# 勿把含 Token 的文件提交进 Git
git remote set-url origin "https://<GITHUB_USERNAME>:<GITHUB_PAT>@github.com/fulldog/ai-agent.git"
```

验证：

```bash
cd /opt/ai-agent
git fetch origin
git log -1 --oneline origin/main
```

> 下文以安装路径 `/opt/ai-agent` 为例，可按实际修改。

### 1.3 创建 `.env`（只做一次）

```bash
cd /opt/ai-agent
cp deploy/docker/env.example .env
vi .env
```

**至少修改：**

| 变量 | 说明 |
|------|------|
| `API_KEYS` | 对外 API Key（请求头 `X-API-Key`） |
| `DEEPSEEK_API_KEY` | 或其它厂商 Key |
| `POSTGRES_PASSWORD` | 数据库密码 |
| `DATABASE_URL` | 与上面用户/密码一致，**主机名必须是 `db`** |

示例：

```env
API_KEYS=prod-change-me
DEEPSEEK_API_KEY=sk-xxxx
POSTGRES_USER=ai_agent
POSTGRES_PASSWORD=请改成强密码
POSTGRES_DB=ai_agent
POSTGRES_PORT=5432
DATABASE_URL=postgres://ai_agent:请改成强密码@db:5432/ai_agent?sslmode=disable
APP_PORT=18090
GOPROXY=https://goproxy.cn,direct
```

> 宿主机若已有旧 Postgres 占用 `5432`，设 `POSTGRES_PORT=5433`；`DATABASE_URL` 里端口仍写 **`5432`**（容器内端口）。

```bash
mkdir -p data/logs data/attachments
chmod +x deploy/docker/update-and-start.sh
```

---

## 2. 首次完整启动

```bash
cd /opt/ai-agent
./deploy/docker/update-and-start.sh
```

脚本会：

1. `git fetch` + `git reset --hard origin/main`（对齐 GitHub 最新）  
2. `docker compose up -d --build`（编译应用镜像并启动 app + db）

验证：

```bash
docker compose ps
curl -sS http://127.0.0.1:18090/health
# 期望: {"status":"ok","db":"up"}

curl -sS -H "X-API-Key: 你的API_KEYS" http://127.0.0.1:18090/api/v1/models
```

---

## 3. 开机 / 每次重启自动：拉代码 → 编译 → 启动

使用 systemd（推荐）。

### 3.1 安装 unit

```bash
# 若安装路径不是 /opt/ai-agent，先改 service 里的路径
sudo cp /opt/ai-agent/deploy/docker/ai-agent.service /etc/systemd/system/ai-agent.service
sudo vi /etc/systemd/system/ai-agent.service   # 核对 WorkingDirectory / ExecStart 路径

sudo systemctl daemon-reload
sudo systemctl enable ai-agent.service
sudo systemctl start ai-agent.service
```

### 3.2 行为说明

| 时机 | 动作 |
|------|------|
| `systemctl start ai-agent` / 开机 | 执行 `update-and-start.sh`：拉 GitHub → 构建 → `up -d` |
| `systemctl restart ai-agent` | 先 `compose stop`，再重新拉代码并编译启动 |
| `systemctl stop ai-agent` | `docker compose stop`（数据卷保留） |

查看：

```bash
sudo systemctl status ai-agent
journalctl -u ai-agent -f
docker compose -f /opt/ai-agent/docker-compose.yml logs -f app
```

### 3.3 手动触发一次更新

```bash
cd /opt/ai-agent
./deploy/docker/update-and-start.sh
# 或
sudo systemctl restart ai-agent
```

---

## 4. 部署脚本做了什么（可直接读源码）

文件：`deploy/docker/update-and-start.sh`

```text
校验 git / docker / .env
 → mkdir data/logs data/attachments
 → git fetch + checkout main + reset --hard origin/main
 → docker compose -f docker-compose.yml up -d --build --remove-orphans
 → 打印 compose ps
```

环境变量（可选）：

| 变量 | 默认 | 含义 |
|------|------|------|
| `DEPLOY_BRANCH` | `main` | 跟踪分支 |
| `DEPLOY_REMOTE` | `origin` | 远程名 |
| `COMPOSE_FILE` | `docker-compose.yml` | Compose 文件 |

**注意：** `git reset --hard` 会丢掉对**已跟踪文件**的本地修改。请只改 `.env`、以及 `data/`。不要直接改已入库文件当生产补丁，应推到 GitHub 再拉。

若 systemd 以 **root** 跑脚本，而 HTTPS 凭证写在普通用户的 `~/.git-credentials`，会导致 `git fetch` 失败。任选其一：

- unit 里加 `User=` 为克隆仓库的那个用户；或  
- 用 root 再执行一次 `git fetch` 并配置 root 的 `credential.helper store`；或  
- remote URL 内嵌 PAT（仅限加固后的专用机）。

---

## 5. 目录与持久化

```text
/opt/ai-agent/                 # git 仓库（会随重启更新）
├── .env                       # 机密，不进 Git
├── data/logs/                 # 应用日志（access/info/error/llm）
├── data/attachments/          # 上传附件
├── docker-compose.yml
├── Dockerfile
└── deploy/docker/
    ├── update-and-start.sh    # 拉代码 + 编译启动
    ├── ai-agent.service       # systemd
    ├── env.example
    └── initdb/01-vector.sql   # 首次初始化启用 vector
```

| 数据 | 是否随 git pull 丢失 |
|------|----------------------|
| `.env` | 否（gitignore） |
| `data/*` | 否（gitignore） |
| Postgres 数据 | 否（Docker volume） |
| 应用代码 / 镜像 | 会更新为 GitHub 最新并重新 build |

---

## 6. 常用运维

```bash
cd /opt/ai-agent

docker compose logs -f app
docker compose logs -f db
ls -l data/logs/

docker compose exec db psql -U ai_agent -d ai_agent
# \dx   应有 vector

docker compose restart app
docker compose down
```

备份数据库：

```bash
docker compose exec -T db pg_dump -U ai_agent ai_agent > /opt/backup/ai_agent-$(date +%F).sql
```

---

## 7. 验证文件分析

```bash
curl -sS -X POST "http://127.0.0.1:18090/api/v1/chat/analyze" \
  -H "X-API-Key: 你的API_KEYS" \
  -F "file=@/path/to/demo.pdf" \
  -F "provider=deepseek" \
  -F 'fields=["合同编号","甲方","乙方"]'
```

更多接口见 [API.md](./API.md)。

---

## 8. 故障排查

| 现象 | 处理 |
|------|------|
| `git fetch` 认证失败 | 检查 PAT 是否过期；`~/.git-credentials` 权限与 **运行用户**是否一致 |
| `Repository not found` | 确认 HTTPS URL、账号对私有仓有读权限 |
| `缺少 .env` | `cp deploy/docker/env.example .env` 并填写 |
| 构建拉不下基础镜像 | 配置 Docker 镜像加速；确认 `GOPROXY` |
| app 起不来 / `db: down` | `docker compose ps`、`logs db` |
| 端口冲突 | `.env` 中改 `APP_PORT` / `POSTGRES_PORT` |
| 重启后代码不是最新 | `journalctl -u ai-agent`；核对 unit 路径与 `User=` |

---

## 9. 生产建议

- [ ] `.env`：`chmod 600 .env`；含 Token 的 `~/.git-credentials` 同样 `600`  
- [ ] PAT 使用**最小权限**、可撤销的 fine-grained / classic token  
- [ ] 不要把 Postgres `5432` 对公网开放  
- [ ] 定期 `pg_dump` + 备份 `data/attachments`  
- [ ] 反向代理 HTTPS + SSE 长超时  

```nginx
location / {
    proxy_pass http://127.0.0.1:18090;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_buffering off;
    proxy_read_timeout 3600s;
}
```

---

## 10. 从零复制清单（HTTPS）

```bash
# 1. 依赖：git、docker、compose 已就绪
sudo mkdir -p /opt
sudo git clone https://github.com/fulldog/ai-agent.git /opt/ai-agent
# 私有仓：sudo git clone "https://USER:PAT@github.com/fulldog/ai-agent.git" /opt/ai-agent
sudo chown -R "$USER":"$USER" /opt/ai-agent
cd /opt/ai-agent

# 2. 配置
cp deploy/docker/env.example .env && vi .env
mkdir -p data/logs data/attachments
chmod +x deploy/docker/update-and-start.sh

# 3. 首次启动
./deploy/docker/update-and-start.sh
curl -sS http://127.0.0.1:18090/health

# 4. 开机自动拉代码并编译
sudo cp deploy/docker/ai-agent.service /etc/systemd/system/
# 若路径不是 /opt/ai-agent，编辑 unit
sudo systemctl daemon-reload
sudo systemctl enable --now ai-agent.service
```

之后每次 **`systemctl restart ai-agent` 或机器重启**，都会经 HTTPS 从 GitHub 拉取 `main` 最新提交并重新编译部署完整栈。
