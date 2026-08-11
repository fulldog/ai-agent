# Docker 完整部署（Git 拉取最新代码 + 编译）

> 仓库（HTTPS）：`https://github.com/fulldog/ai-agent.git`（默认分支 `main`）  
> 前提：服务器已安装 **git**、**Docker**、**Docker Compose V2**  
> 行为：重启时 `git pull` → 编译 → 启动；**API Key / 库地址写在宿主机 `configs/config.yaml`，挂载进容器直接使用**

| 容器 | 说明 |
|------|------|
| `ai-agent-app` | 挂载 `configs/config.yaml` → `/app/configs/config.yaml` |
| `ai-agent-db` | 仅当 5432 空闲时启动（PG16+pgvector） |
| `ai-agent-ollama` | 可选；RAG Embedding（`docker-compose.ollama.yml`） |

**配置方式（推荐）**

```bash
cp configs/config.docker.yaml configs/config.yaml
vi configs/config.yaml   # 填写 llm.providers.*.api_key、database.dsn、auth.api_keys
```

`configs/config.yaml` 已在 `.gitignore`，`git pull` **不会覆盖**你的密钥。

**数据库策略（默认 auto）**

| 情况 | 行为 |
|------|------|
| 5432 已有 Postgres | 不新建库容器；按 config 里的 dsn 连接（运行时主机改写为 `host.docker.internal`）；自动安装/启用 pgvector |
| 5432 空闲 | 启动内置 `ai-agent-db`；运行时 dsn 主机改写为 `db` |

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

### 1.3 创建配置文件（只做一次，密钥写这里）

```bash
cd /opt/ai-agent
cp configs/config.docker.yaml configs/config.yaml
vi configs/config.yaml
```

**必改示例：**

```yaml
auth:
  api_keys:
    - "你的对外API-Key"

database:
  enabled: true
  # 已有宿主机/其它容器 Postgres（映射 5432）时推荐：
  dsn: "postgres://用户:密码@host.docker.internal:5432/ai_agent?sslmode=disable"
  # 若用本项目内置库，写成：
  # dsn: "postgres://ai_agent:ai_agent_dev@db:5432/ai_agent?sslmode=disable"

llm:
  providers:
    deepseek:
      api_key: "sk-..."
    qwen:
      api_key: "sk-..."   # provider=qwen / 通义文件分析时需要
```

改完密钥后只需：

```bash
docker compose restart app
# 或
bash deploy/docker/update-and-start.sh
```

可选：`.env` 仅用于 `DB_MODE` / `APP_PORT` / 内置库账号等，**不必**再配 `DASHSCOPE_API_KEY`。

```bash
mkdir -p data/logs data/attachments
# 用 bash 调用脚本即可，不要 chmod +x（会改 filemode，git status 显示脏）
```

### 1.4 单独检查 / 自动安装 pgvector

```bash
cd /opt/ai-agent
bash deploy/docker/ensure-pgvector.sh "postgres://postgres:密码@127.0.0.1:5432/ai_agent?sslmode=disable"
```

行为：

1. 能连上库且已有 `vector` 扩展文件 → 直接 `CREATE EXTENSION IF NOT EXISTS vector`  
2. **没有扩展文件**、且库在本机 Docker（端口映射到 `5432`）→ **自动在该容器内编译安装 pgvector**，再启用扩展  
3. 库不在本机 / 非 Docker / 容器无包管理器 → 报错并提示改用 `pgvector/pgvector` 镜像  

可选环境变量：`PGVECTOR_REF=v0.8.0`（源码版本）、`PG_CLIENT_IMAGE=postgres:16`、`PGVECTOR_SKIP_INSTALL=1`（只检查不安装）。

---

## 2. 首次完整启动

```bash
cd /opt/ai-agent
./deploy/docker/update-and-start.sh
```

脚本会：

1. `git fetch` + `git reset --hard origin/main`  
2. 按 `DB_MODE` / `5432` 是否占用决定：**复用外部库**或**启动内置 db**  
3. `ensure-pgvector.sh` 检查并启用 `vector`  
4. `docker compose up -d --build` 启动应用（及可选的内置库）

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
# 仅拉代码 + 重编译 + 重启 app（不动 db）
bash deploy/docker/restart-app.sh

# 或完整部署（含数据库策略 / pgvector）
bash deploy/docker/update-and-start.sh
# 或
sudo systemctl restart ai-agent
```

---

## 4. 部署脚本做了什么（可直接读源码）

文件：`deploy/docker/update-and-start.sh`

```text
校验 git / docker / .env
 → git fetch + reset --hard origin/main
 → 判断 DB_MODE / 5432 端口 / ai-agent-db 是否已运行
 → 外部库：ensure-pgvector → 只启动 app
 → 内置库：启动 app+db → ensure-pgvector
```

相关文件：

| 文件 | 作用 |
|------|------|
| `docker-compose.yml` | 仅应用 |
| `docker-compose.db.yml` | 内置 Postgres+pgvector（按需叠加） |
| `docker-compose.ollama.yml` | 可选 Ollama Embedding（按需叠加） |
| `deploy/docker/ensure-pgvector.sh` | 探测连接 + 检查/启用 vector |
| `deploy/docker/restart-app.sh` | 拉代码 → 编译镜像 → 仅重启 app |
| `deploy/docker/update-and-start.sh` | 拉代码 + 数据库策略 + 编译启动 |

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

## 5. Embedding / Ollama（Docker 推荐）

RAG、语料入库、`reindex` 需要 Embedding 服务。默认模型：`nomic-embed-text`（维度 **768**，须与 `embed.dimensions` / 库表一致）。

推荐用 Compose 叠加文件启动 **容器内 Ollama**（与 app 同网络 `ai-agent-net`），不必在宿主机再装一份。

### 5.1 启动 Ollama 容器

```bash
cd /opt/ai-agent

# 仅起 Ollama（app / db 可已在跑）
docker compose -f docker-compose.yml -f docker-compose.ollama.yml up -d ollama

# 或与 app 一起
docker compose -f docker-compose.yml -f docker-compose.ollama.yml up -d
```

首次拉取模型（体积不大，需能访问模型下载源）：

```bash
docker exec -it ai-agent-ollama ollama pull nomic-embed-text
docker exec -it ai-agent-ollama ollama list
```

自检：

```bash
curl -sS http://127.0.0.1:11434/api/tags
# 容器网络内（从 app 视角）：http://ollama:11434
```

模型数据落在 Docker volume `ai_agent_ollama`，`compose down` 默认不删 volume。

### 5.2 配置 `embed.base_url`

编辑宿主机 `configs/config.yaml`：

```yaml
embed:
  base_url: "http://ollama:11434/v1"   # 与 ai-agent-app 同 compose 网络时用服务名
  api_key: ""
  model: "nomic-embed-text"
  dimensions: 768
```

| 场景 | `embed.base_url` |
|------|------------------|
| Compose 内 `ai-agent-ollama`（推荐） | `http://ollama:11434/v1` |
| 宿主机安装的 Ollama | `http://host.docker.internal:11434/v1` |
| 本机直接跑 Go（非 Docker） | `http://127.0.0.1:11434/v1` |

改完配置后重建 / 重启 app：

```bash
bash deploy/docker/restart-app.sh
# 或
docker compose up -d --no-deps --force-recreate app
```

`configs/config.docker.yaml` 默认已按 Compose Ollama 写成 `http://ollama:11434/v1`。

### 5.3 GPU（可选）

宿主机已装 [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html) 时，编辑 `docker-compose.ollama.yml`，取消 `deploy.resources...devices` 注释后：

```bash
docker compose -f docker-compose.yml -f docker-compose.ollama.yml up -d ollama
```

仅做 Embedding 时 CPU 通常够用。

### 5.4 常用命令

```bash
docker compose -f docker-compose.yml -f docker-compose.ollama.yml logs -f ollama
docker compose -f docker-compose.yml -f docker-compose.ollama.yml stop ollama
docker compose -f docker-compose.yml -f docker-compose.ollama.yml start ollama
```

不用 RAG 时可不起 Ollama；聊天 / analyze 不依赖 Embedding。

### 5.5 备选：宿主机安装 Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama pull nomic-embed-text
```

此时 `embed.base_url` 用 `http://host.docker.internal:11434/v1`，**不要**再起 `ai-agent-ollama`（避免抢 11434）。

---

## 6. 目录与持久化

```text
/opt/ai-agent/                 # git 仓库（会随重启更新）
├── .env                       # 机密，不进 Git
├── data/logs/                 # 应用日志（access/info/error/llm）
├── data/attachments/          # 上传附件
├── docker-compose.yml
├── docker-compose.db.yml
├── docker-compose.ollama.yml  # 可选 Embedding
├── Dockerfile
└── deploy/docker/
    ├── update-and-start.sh    # 拉代码 + 编译启动
    ├── restart-app.sh
    ├── ai-agent.service       # systemd
    ├── env.example
    └── initdb/01-vector.sql   # 首次初始化启用 vector
```

| 数据 | 是否随 git pull 丢失 |
|------|----------------------|
| `.env` | 否（gitignore） |
| `data/*` | 否（gitignore） |
| Postgres 数据 | 否（Docker volume） |
| Ollama 模型 | 否（volume `ai_agent_ollama`） |
| 应用代码 / 镜像 | 会更新为 GitHub 最新并重新 build |

---

## 7. 常用运维

```bash
cd /opt/ai-agent

docker compose logs -f app
docker compose -f docker-compose.yml -f docker-compose.db.yml logs -f db
docker compose -f docker-compose.yml -f docker-compose.ollama.yml logs -f ollama
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

## 8. 验证文件分析

```bash
curl -sS -X POST "http://127.0.0.1:18090/api/v1/chat/analyze" \
  -H "X-API-Key: 你的API_KEYS" \
  -F "file=@/path/to/demo.pdf" \
  -F "provider=deepseek" \
  -F 'fields=["合同编号","甲方","乙方"]'
```

更多接口见 [API.md](./API.md)。

---

## 9. 故障排查

| 现象 | 处理 |
|------|------|
| `git fetch` 认证失败 | 检查 PAT / `~/.git-credentials` 与运行用户 |
| 外部库连不上 | 检查 `EXTERNAL_DATABASE_URL`、库是否已 `CREATE DATABASE` |
| 自动安装 pgvector 失败 | 看脚本日志：容器是否映射端口、能否 apt/apk；或换镜像 `pgvector/pgvector:pg16` |
| 提示非本机无法安装 | 连接串须为 `127.0.0.1`；远端库需在库所在机器安装扩展 |
| `缺少 .env` | `cp deploy/docker/env.example .env` |
| 构建拉不下镜像 | Docker 镜像加速；`GOPROXY`；可选 `PG_CLIENT_IMAGE=pgvector/pgvector:pg16` |
| app `db: down` | 外部库防火墙/`pg_hba.conf` 是否允许 Docker 网桥访问；试 `host.docker.internal` |
| 端口冲突 | `DB_MODE=auto` 应复用；或 `embedded` 时改 `POSTGRES_PORT` |
| RAG / reindex 连不上 Embedding | 确认已起 `ai-agent-ollama`；`embed.base_url` 用 `http://ollama:11434/v1`（勿用容器内 `localhost`） |
| `nomic-embed-text` 找不到 | `docker exec -it ai-agent-ollama ollama pull nomic-embed-text` |
| 11434 端口冲突 | 改 `OLLAMA_PORT`，或停掉宿主机 Ollama / 旧容器 |

---

## 10. 生产建议

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

## 11. 从零复制清单（HTTPS + 配置文件挂载）

```bash
sudo mkdir -p /opt
sudo git clone https://github.com/fulldog/ai-agent.git /opt/ai-agent
sudo chown -R "$USER":"$USER" /opt/ai-agent
cd /opt/ai-agent

cp configs/config.docker.yaml configs/config.yaml
vi configs/config.yaml          # 填 api_key、dsn、api_keys；RAG 用 embed.base_url=http://ollama:11434/v1
mkdir -p data/logs data/attachments

bash deploy/docker/update-and-start.sh

# 需要 RAG 时再起 Ollama 并拉模型
docker compose -f docker-compose.yml -f docker-compose.ollama.yml up -d ollama
docker exec -it ai-agent-ollama ollama pull nomic-embed-text

curl -sS http://127.0.0.1:18090/health

sudo cp deploy/docker/ai-agent.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ai-agent.service
```

之后改密钥只需改 `configs/config.yaml`，然后 `docker compose restart app`。
