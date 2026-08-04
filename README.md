# AI Agent

独立 Go AI Agent 后端：DeepSeek 对话与 Tool Calling、RAG（PostgreSQL + pgvector）、API Key 鉴权、完整请求日志、Prometheus 监控。  
M4：CloudWeGo Eino DeepSeek ChatModel + Agent 工具循环。

模块名：`github.com/webapp/go-app/ai-agent`（与仓库内 [`agent/`](../agent/) 并列，互不依赖）。

## 环境要求

| 依赖 | 说明 |
|------|------|
| Go | **1.25.5**（本机路径示例：`D:\gosdk\go1.25.5`） |
| PostgreSQL 16+ | Community + [pgvector](https://github.com/pgvector/pgvector)（安装见 [docs/POSTGRES.md](docs/POSTGRES.md)） |
| DeepSeek API Key | 对话 / Agent |
| Ollama（可选） | 默认 Embedding：`nomic-embed-text` |

**数据库安装与 `DATABASE_URL` 配置**：完整步骤见 [docs/POSTGRES.md](docs/POSTGRES.md)（Windows 安装 PostgreSQL、编译 pgvector、建库建用户、环境变量/配置文件）。

若 PATH 仍指向旧 Go，构建前先切换：

```powershell
$env:PATH = "D:\gosdk\go1.25.5\bin;" + $env:PATH
$env:GOROOT = "D:\gosdk\go1.25.5"
go version   # 应显示 go1.25.5
```

## 快速开始

```powershell
cd c:\webapp\go-app\ai-agent
copy configs\config.example.yaml configs\config.yaml

# 配置密钥与数据库
$env:DEEPSEEK_API_KEY = "sk-..."
$env:DATABASE_URL = "postgres://ai_agent:password@127.0.0.1:5432/ai_agent?sslmode=disable"

go run ./cmd/server -config configs/config.yaml
```

- 服务：`http://localhost:18090`
- 健康检查：`GET /health`
- 指标：`GET /metrics`
- API 前缀：`/api/v1`（需 Header `X-API-Key`）
- 文件日志：工作目录下 `logs/ai-agent-YYYY-MM-DD.log`，按日切换；单文件超过 **100MB** 自动切分（配置见 `log.*`）。`server.mode: release` 时默认**不**打 stdout，只写文件

默认示例 Key（见配置）：`change-me-api-key`。

## 文档

| 文档 | 说明 |
|------|------|
| [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md) | 主规划书 |
| [docs/API.md](docs/API.md) | REST / SSE 接口 |
| [docs/POSTGRES.md](docs/POSTGRES.md) | PostgreSQL + pgvector 安装与 DATABASE_URL |
| [docs/DB_SCHEMA.md](docs/DB_SCHEMA.md) | 表结构 / 向量列与索引 |
| [configs/config.example.yaml](configs/config.example.yaml) | 配置样例 |
| [deploy/prometheus/scrape.example.yml](deploy/prometheus/scrape.example.yml) | Prometheus 抓取示例 |

## 技术栈

| 组件 | 选型 |
|------|------|
| HTTP | Gin |
| 日志 | zap |
| DB / 向量 | PostgreSQL + pgvector |
| LLM | DeepSeek（Eino ChatModel + 服务层 HTTP 流式/工具循环） |
| Agent | Tool Calling（`knowledge_search`、`current_time`） |
| Embedding | OpenAI 兼容（默认 Ollama） |
| 鉴权 | `X-API-Key` |
| 监控 | Prometheus `/metrics` |

## 实现里程碑

| 里程碑 | 状态 |
|--------|------|
| M0 规划文档 | 完成 |
| M1 脚手架 / 鉴权 / 请求日志 / metrics | 完成 |
| M2 会话 + SSE 聊天 | 完成 |
| M3 语料 + pgvector RAG | 完成 |
| M4 Eino DeepSeek + Agent | 完成 |
| M5 日志查询 / 测试 / README | 完成 |

## 常用接口示例

```powershell
$H = @{ "X-API-Key" = "change-me-api-key"; "Content-Type" = "application/json" }

# 创建会话
Invoke-RestMethod -Method POST -Uri http://localhost:18090/api/v1/conversations -Headers $H -Body '{"title":"demo"}'

# 流式聊天（需 conversation_id）
# POST /api/v1/chat/completions/stream
```

## 与 `agent/` 的差异

- 本项目：zap、PostgreSQL + pgvector、API Key、Prometheus、Eino DeepSeek
- `agent/`：自定义日志、SQLite、JWT 多租户、文档复核管理后台
