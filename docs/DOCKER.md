# Docker 完整部署（Git 拉取最新代码 + 编译）

> 仓库：`git@github.com:fulldog/ai-agent.git`（默认分支 `main`）  
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

### 1.2 配置 GitHub 拉取权限

本仓库远程为 SSH：`git@github.com:fulldog/ai-agent.git`。

```bash
# 生成部署用密钥（若尚无）
ssh-keygen -t ed25519 -C "ai-agent-deploy" -f ~/.ssh/ai_agent_deploy -N ""

# 把 ~/.ssh/ai_agent_deploy.pub 加到 GitHub：
# 仓库 Settings → Deploy keys → Add deploy key（只读即可）
# 或加到有权限的个人/机器账号 SSH Keys

# ~/.ssh/config 示例
cat >> ~/.ssh/config <<'EOF'
Host github.com
  HostName github.com
  User git
  IdentityFile ~/.ssh/ai_agent_deploy
  StrictHostKeyChecking accept-new
EOF
chmod 600 ~/.ssh/config

ssh -T git@github.com
# 成功会提示 Hi fulldog/... 
```

若只能用 HTTPS + Token：

```bash
git clone https://<TOKEN>@github.com/fulldog/ai-agent.git /opt/ai-agent
```

下文以 `/opt/ai-agent` 为例，可按实际路径修改。

### 1.3 克隆仓库

```bash
sudo mkdir -p /opt
sudo git clone git@github.com:fulldog/ai-agent.git /opt/ai-agent
# 若目录属主不是当前用户：
sudo chown -R "$USER":"$USER" /opt/ai-agent
cd /opt/ai-agent
git checkout main
```

### 1.4 创建 `.env`（只做一次）

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

### 3.3 手动触发一次更新（不停机策略下的热更）

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

**注意：** `git reset --hard` 会丢掉对**已跟踪文件**的本地修改。请只改 `.env`、挂载的自定义配置（若自行挂载且未提交）、以及 `data/`。不要直接改已入库的 `Dockerfile` 等作为生产补丁，应改 GitHub 再拉。

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
| Postgres 数据 | 否（Docker volume `ai-agent_ai_agent_pgdata`） |
| 应用代码 / 镜像 | 会更新为 GitHub 最新并重新 build |

---

## 6. 常用运维

```bash
cd /opt/ai-agent

# 日志
docker compose logs -f app
docker compose logs -f db
ls -l data/logs/

# 进库
docker compose exec db psql -U ai_agent -d ai_agent
# \dx   应有 vector

# 仅重启容器（不拉代码）
docker compose restart app

# 停栈（保留数据）
docker compose down

# 危险：删库数据卷
docker compose down -v
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
| `git fetch` 失败 | 检查 Deploy Key / `ssh -T git@github.com` |
| `缺少 .env` | `cp deploy/docker/env.example .env` 并填写 |
| 构建拉不下基础镜像 | 配置 Docker 镜像加速；确认 `GOPROXY` |
| app 起不来 / `db: down` | `docker compose ps`、`logs db`；等 healthcheck 通过 |
| 端口冲突 | `.env` 中改 `APP_PORT` / `POSTGRES_PORT` |
| 重启后代码不是最新 | `journalctl -u ai-agent` 看是否执行了脚本；确认 `WorkingDirectory` 正确 |
| 本地改过代码被冲掉 | 预期行为；改 GitHub 或只改 `.env` |

---

## 9. 生产建议

- [ ] `.env` 权限：`chmod 600 .env`  
- [ ] 不要把 Postgres `5432` 对公网开放（可删掉 compose 里 db 的 `ports`）  
- [ ] 定期 `pg_dump` + 备份 `data/attachments`  
- [ ] 反向代理 HTTPS + SSE 长超时（见下方）  
- [ ] 部署机只读 Deploy Key，勿放个人高权限 token  

Nginx 反代摘要：

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

## 10. 从零复制清单

```bash
# 1. 依赖：git、docker、compose 已就绪
sudo mkdir -p /opt && sudo git clone git@github.com:fulldog/ai-agent.git /opt/ai-agent
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
# 若路径不是 /opt/ai-agent，编辑 unit 中的路径
sudo systemctl daemon-reload
sudo systemctl enable --now ai-agent.service
```

之后每次 **`systemctl restart ai-agent` 或机器重启**，都会从 GitHub 拉取 `main` 最新提交并重新编译部署完整栈。
