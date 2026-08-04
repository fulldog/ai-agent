# PostgreSQL + pgvector 安装与配置

> 适用本项目 `ai-agent`。PostgreSQL Community 与 pgvector 均为免费开源。  
> **Linux / Windows 均可**；下文先给 Linux 速查，再详述 Windows。  
> 关联：[DB_SCHEMA.md](./DB_SCHEMA.md)、[README.md](../README.md)、[`configs/config.example.yaml`](../configs/config.example.yaml)

---

## 1. 目标结果

安装完成后应满足：

| 项 | 建议值 |
|----|--------|
| PostgreSQL | 16 或 17（Community） |
| 扩展 | `vector`（pgvector） |
| 数据库 | `ai_agent` |
| 用户 | `ai_agent` |
| 端口 | `5432` |
| 连接串 | 见下文 `DATABASE_URL` |

应用侧只需配置好 DSN；启动时会自动执行：

```sql
CREATE EXTENSION IF NOT EXISTS vector;
```

并做表迁移（`database.auto_migrate: true`）。

---

## 2. Linux 速查（生产常用）

以 Debian/Ubuntu 为例（版本号按发行版调整）：

```bash
# PostgreSQL（示例：16）
sudo apt update
sudo apt install -y postgresql postgresql-contrib

# pgvector：优先用发行版包（名称因版本而异）
# Ubuntu 24.04+ 常见：
sudo apt install -y postgresql-16-pgvector
# 若无对应包，按官方文档源码编译：https://github.com/pgvector/pgvector#installation

sudo -u postgres psql <<'SQL'
CREATE USER ai_agent WITH PASSWORD 'ai_agent_dev';
CREATE DATABASE ai_agent OWNER ai_agent;
\c ai_agent
CREATE EXTENSION IF NOT EXISTS vector;
GRANT ALL ON SCHEMA public TO ai_agent;
SQL

export DATABASE_URL='postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable'
```

RHEL/Rocky：用 `dnf install postgresql-server` + 对应 `pgvector` 包，或源码编译扩展。连接串格式与 Windows 相同。

---

## 3. 安装 PostgreSQL（Windows）

### 3.1 图形安装（推荐）

1. 打开下载页：[PostgreSQL Windows 安装包（EDB）](https://www.postgresql.org/download/windows/)
2. 安装 **PostgreSQL 16** 或 **17**（x64）
3. 安装向导注意：
   - **端口**：`5432`（勿与其他服务冲突）
   - **超级用户**：`postgres`
   - **密码**：自行设置并妥善保存（下文称 `POSTGRES_SUPER_PASSWORD`）
   - 组件：至少勾选 **PostgreSQL Server**、**Command Line Tools**
   - Stack Builder 可跳过
4. 安装完成后确认服务已启动（服务名类似 `postgresql-x64-17`）

### 3.2 winget 安装（可选）

```powershell
winget install --id PostgreSQL.PostgreSQL.17 -e --accept-package-agreements --accept-source-agreements
```

若静默参数失败，改用图形安装即可。安装后把 `bin` 加入 PATH，例如：

```powershell
# 按实际安装目录调整版本号
$env:PATH = "C:\Program Files\PostgreSQL\17\bin;" + $env:PATH
psql --version
```

建议将上述 `bin` 永久加入系统环境变量 PATH。

### 3.3 验证

```powershell
psql -U postgres -h 127.0.0.1 -p 5432 -c "SELECT version();"
```

按提示输入超级用户密码。

---

## 4. 安装 pgvector（Windows）

官方扩展：[pgvector](https://github.com/pgvector/pgvector)。Windows 需编译安装（需与已装 PostgreSQL 主版本一致）。

### 4.1 前置条件

- 已安装对应版本的 PostgreSQL（含开发头文件，官方安装包一般自带）
- [Visual Studio 2022](https://visualstudio.microsoft.com/)（含「使用 C++ 的桌面开发」）或 Build Tools
- Git（可选，用于拉取源码）

在 **x64 Native Tools Command Prompt for VS 2022**（或已加载 VS 环境的 PowerShell）中操作。

### 4.2 编译安装

```powershell
# 1) 设置 PostgreSQL 路径（按本机版本修改）
$env:PGROOT = "C:\Program Files\PostgreSQL\17"

# 2) 拉取源码
cd D:\src   # 任意工作目录
git clone --branch v0.8.0 https://github.com/pgvector/pgvector.git
cd pgvector

# 3) 编译并安装到 PostgreSQL 目录（需管理员权限）
nmake /F Makefile.win
nmake /F Makefile.win install
```

成功后，扩展文件会出现在：

- `$env:PGROOT\share\extension\vector.control`
- `$env:PGROOT\lib\vector.dll`

### 4.3 验证扩展文件

```powershell
Test-Path "$env:PGROOT\share\extension\vector.control"
Test-Path "$env:PGROOT\lib\vector.dll"
```

均为 `True` 即安装到位。

### 4.4 在数据库中启用

见下一节创建库后执行：

```sql
CREATE EXTENSION IF NOT EXISTS vector;
SELECT extversion FROM pg_extension WHERE extname = 'vector';
```

---

## 5. 创建业务库与用户

使用超级用户执行（PowerShell）：

```powershell
$env:PATH = "C:\Program Files\PostgreSQL\17\bin;" + $env:PATH
$env:PGPASSWORD = "<POSTGRES_SUPER_PASSWORD>"

# 创建角色与数据库（密码请自行修改）
psql -U postgres -h 127.0.0.1 -p 5432 -v ON_ERROR_STOP=1 -c "CREATE USER ai_agent WITH PASSWORD 'ai_agent_dev';"
psql -U postgres -h 127.0.0.1 -p 5432 -v ON_ERROR_STOP=1 -c "CREATE DATABASE ai_agent OWNER ai_agent;"
psql -U postgres -h 127.0.0.1 -p 5432 -d ai_agent -v ON_ERROR_STOP=1 -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql -U postgres -h 127.0.0.1 -p 5432 -d ai_agent -v ON_ERROR_STOP=1 -c "GRANT ALL ON SCHEMA public TO ai_agent;"

# 清理环境变量中的密码
Remove-Item Env:PGPASSWORD
```

若提示角色/库已存在，可忽略对应错误或先检查：

```powershell
psql -U postgres -h 127.0.0.1 -c "\du"
psql -U postgres -h 127.0.0.1 -c "\l"
```

用业务用户自测：

```powershell
$env:PGPASSWORD = "ai_agent_dev"
psql -U ai_agent -h 127.0.0.1 -d ai_agent -c "SELECT extname, extversion FROM pg_extension WHERE extname='vector';"
Remove-Item Env:PGPASSWORD
```

---

## 6. 配置 DATABASE_URL（本项目）

### 6.1 连接串格式

```text
postgres://USER:PASSWORD@HOST:PORT/DBNAME?sslmode=disable
```

本机开发示例：

```text
postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable
```

> 密码含特殊字符时需 [URL 编码](https://developer.mozilla.org/en-US/docs/Glossary/Percent-encoding)（如 `@` → `%40`）。

### 6.2 方式 A：环境变量（推荐）

优先级：`DATABASE_URL` > `PG_DSN` > `configs/config.yaml` 中的 `database.dsn`。

```bash
# Linux / macOS
export DATABASE_URL='postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable'
# 可选写入 ~/.bashrc 或 systemd Environment=
```

```powershell
# Windows PowerShell
$env:DATABASE_URL = "postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable"
```

Windows 可选长期写入用户环境变量：

```powershell
[System.Environment]::SetEnvironmentVariable(
  "DATABASE_URL",
  "postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable",
  "User"
)
```

新开终端后生效。

### 6.3 方式 B：配置文件

```bash
# Linux / macOS
cp configs/config.example.yaml configs/config.yaml
```

```powershell
# Windows
cd c:\webapp\go-app\ai-agent
copy configs\config.example.yaml configs\config.yaml
```

编辑 `configs/config.yaml`：

```yaml
database:
  dsn: "postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable"
  max_open_conns: 20
  max_idle_conns: 5
  auto_migrate: true
```

`configs/config.yaml` 已在 `.gitignore` 中，勿把真实密码提交到仓库。

### 6.4 启动应用验证

```bash
# Linux / macOS
export DATABASE_URL='postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable'
export DEEPSEEK_API_KEY='sk-...'   # 可选；未设置时跳过 ChatModel
go run ./cmd/server -config configs/config.yaml
curl -s http://localhost:18090/health
```

```powershell
# Windows
$env:PATH = "D:\gosdk\go1.25.5\bin;" + $env:PATH
$env:GOROOT = "D:\gosdk\go1.25.5"
$env:DATABASE_URL = "postgres://ai_agent:ai_agent_dev@127.0.0.1:5432/ai_agent?sslmode=disable"
$env:DEEPSEEK_API_KEY = "sk-..."

cd c:\webapp\go-app\ai-agent
go run ./cmd/server -config configs/config.yaml
```

成功标志：

- 进程监听 `:18090`
- `GET http://127.0.0.1:18090/health` 返回 `"db":"up"`
- 库中出现业务表，且 `chunks` 表具备 `embedding vector(...)` 列

```powershell
Invoke-RestMethod http://127.0.0.1:18090/health
```

---

## 7. 向量索引（可选）

数据量较小时可不建索引；语料增多后建议在库中执行（与配置 `rag.vector_index` 一致）：

```sql
-- HNSW（推荐）
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw
ON chunks USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- 或 IVFFlat
-- CREATE INDEX IF NOT EXISTS idx_chunks_embedding_ivfflat
-- ON chunks USING ivfflat (embedding vector_cosine_ops)
-- WITH (lists = 100);
```

应用在索引语料时也会按配置尝试创建索引（见 `internal/database`）。

---

## 8. 常见问题

| 现象 | 处理 |
|------|------|
| `psql` 不是内部或外部命令 | 将 `C:\Program Files\PostgreSQL\<版本>\bin` 加入 PATH |
| 连接被拒绝 / 超时 | 检查 Windows 服务是否运行；防火墙是否放行 5432 |
| `could not open extension control file ... vector.control` | pgvector 未安装到当前 PostgreSQL 目录，重做第 3 节 |
| `permission denied to create extension vector` | 用超级用户在目标库执行 `CREATE EXTENSION`，或给业务用户授权 |
| `password authentication failed` | 核对用户/密码；URL 中特殊字符需编码 |
| 端口被占用 | 安装时改端口，并同步修改 `DATABASE_URL` 的端口 |
| Embedding 维度不一致 | `embed.dimensions` 必须与建表时 `vector(n)` 一致，换模型需迁移 |

---

## 9. 清单（安装自检）

- [ ] PostgreSQL 服务运行中，`psql --version` 正常  
- [ ] `vector.control` / `vector.dll` 已安装到 `$PGROOT`  
- [ ] 数据库 `ai_agent`、用户 `ai_agent` 已创建  
- [ ] `\dx` 可见 `vector` 扩展  
- [ ] 已设置 `DATABASE_URL` 或 `config.yaml` → `database.dsn`  
- [ ] `GET /health` 返回 `db: up`  
