# AI Agent

独立 Go AI Agent 后端：DeepSeek 对话与 Tool Calling、RAG（PostgreSQL + pgvector）、API Key 鉴权、完整请求日志、Prometheus 监控。  
M4：CloudWeGo Eino DeepSeek ChatModel + Agent 工具循环。

模块名：`github.com/webapp/go-app/ai-agent`。默认端口 `:18090`。

## 环境要求

| 依赖 | 说明 |
|------|------|
| Go | **1.25.5**（Linux / Windows 均可；Windows 路径示例：`D:\gosdk\go1.25.5`） |
| PostgreSQL 16+ | Community + [pgvector](https://github.com/pgvector/pgvector)（安装见 [docs/POSTGRES.md](docs/POSTGRES.md)） |
| DeepSeek API Key | 对话 / Agent |
| Ollama（可选） | 默认 Embedding：`nomic-embed-text` |
| Tesseract OCR | 图片 OCR；扫描版 PDF OCR（见下方「OCR 插件」） |
| Poppler `pdftoppm` | 仅扫描版 PDF 需要（转图片后再 OCR） |

**数据库安装与 `DATABASE_URL` 配置**：见 [docs/POSTGRES.md](docs/POSTGRES.md)（含 Windows 详解；Linux 可用发行版包 + `postgresql-xx-pgvector` / 源码编译）。

### OCR 插件（文档上传）

上传 `POST /api/v1/corpora/{id}/documents` 支持 PDF / Word(docx) / 图片。OCR 依赖本机外部程序（无 CGO），**Linux / Windows 均可**，配置写可执行文件名或绝对路径即可。

| 插件 | 用途 | 是否必须 | Linux | Windows |
|------|------|----------|-------|---------|
| **Tesseract OCR** | 图片 / 扫描 PDF OCR | 图片、扫描 PDF 时必须 | `apt install tesseract-ocr tesseract-ocr-chi-sim`（或 `yum`/`dnf` 等价包） | [UB Mannheim](https://github.com/UB-Mannheim/tesseract/wiki)，勾选 **chi_sim** |
| **Poppler（`pdftoppm`）** | 扫描 PDF 转 PNG | 仅扫描版 PDF | `apt install poppler-utils` | [poppler-windows](https://github.com/oschwartz10612/poppler-windows/releases)，`Library\bin` 加入 PATH |

纯文字 PDF、`.docx`、纯文本 **不需要** 上述 OCR 插件。旧版 `.doc` 不支持，请另存为 `.docx`。

```bash
# Linux / macOS / Git Bash（命令相同）
tesseract --version
pdftoppm -v
which tesseract pdftoppm
```

配置示例（`configs/config.yaml`）：

```yaml
ocr:
  enabled: true
  # Linux 装好后一般直接用命令名；也可写绝对路径如 /usr/bin/tesseract
  tesseract_path: tesseract
  languages: chi_sim+eng
  pdftoppm_path: pdftoppm          # Linux: /usr/bin/pdftoppm；Windows 可写 .exe 绝对路径
  min_pdf_text_len: 40
  timeout_seconds: 180
```

更细说明见 [docs/EXTRACT.md](docs/EXTRACT.md)。

**Windows** 若 PATH 仍指向旧 Go，构建前先切换：

```powershell
$env:PATH = "D:\gosdk\go1.25.5\bin;" + $env:PATH
$env:GOROOT = "D:\gosdk\go1.25.5"
go version   # 应显示 go1.25.5
```

## 快速开始

**Linux / macOS：**

```bash
cd /path/to/ai-agent
cp configs/config.example.yaml configs/config.yaml

export DEEPSEEK_API_KEY="sk-..."
export DATABASE_URL="postgres://ai_agent:password@127.0.0.1:5432/ai_agent?sslmode=disable"

go run ./cmd/server -config configs/config.yaml
# 生产可用：go build -o bin/ai-agent ./cmd/server && ./bin/ai-agent -config configs/config.yaml
```

**Windows（PowerShell）：**

```powershell
cd c:\webapp\go-app\ai-agent
copy configs\config.example.yaml configs\config.yaml

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
| [docs/API.md](docs/API.md) | REST / SSE 接口说明 |
| [docs/openapi.yaml](docs/openapi.yaml) | **OpenAPI 3.0（可直接导入 Apifox）** |
| [docs/EXTRACT.md](docs/EXTRACT.md) | PDF/Word/图片 OCR 与本机依赖 |
| [docs/AGENT_TOOLS.md](docs/AGENT_TOOLS.md) | **Agent 工具规范**（新增 Tool、中文描述、注册清单） |
| [docs/POSTGRES.md](docs/POSTGRES.md) | PostgreSQL + pgvector 安装与 DATABASE_URL |
| [docs/DB_SCHEMA.md](docs/DB_SCHEMA.md) | 表结构 / 向量列与索引 |
| [configs/config.example.yaml](configs/config.example.yaml) | 配置样例 |
| [deploy/prometheus/scrape.example.yml](deploy/prometheus/scrape.example.yml) | Prometheus 抓取示例 |
| 本文件 [附录 A](#附录-a：rag-参数调教说明) | RAG 分块 / top_k / 索引 / Embedding 调教 |

## 技术栈

| 组件 | 选型 |
|------|------|
| HTTP | Gin |
| 日志 | zap |
| DB / 向量 | PostgreSQL + pgvector |
| LLM | DeepSeek（Eino ChatModel + 服务层 HTTP 流式/工具循环） |
| Agent | Tool Calling（`knowledge_search`、`current_time`、`calculator`） |
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

**Linux / macOS：**

```bash
curl -s -X POST http://localhost:18090/api/v1/conversations \
  -H "X-API-Key: change-me-api-key" -H "Content-Type: application/json" \
  -d '{"title":"demo"}'
```

**Windows（PowerShell）：**

```powershell
$H = @{ "X-API-Key" = "change-me-api-key"; "Content-Type" = "application/json" }

# 创建会话
Invoke-RestMethod -Method POST -Uri http://localhost:18090/api/v1/conversations -Headers $H -Body '{"title":"demo"}'

# 流式聊天（需 conversation_id）
# POST /api/v1/chat/completions/stream
```

---

## 附录 A：RAG 参数调教说明

配置文件对应块：`embed`、`rag`（见 `configs/config.example.yaml`）。聊天/Agent 请求里的 `rag.top_k` / `corpus_id` 可覆盖默认检索条数。

### A.1 参数一览

| 配置项 | 默认 | 作用 |
|--------|------|------|
| `embed.model` | `nomic-embed-text` | Embedding 模型名（需与 Ollama/兼容端点一致） |
| `embed.dimensions` | `768` | 向量维度；必须与模型输出一致，且与库表 `chunks.embedding vector(n)` 一致 |
| `embed.base_url` | `http://localhost:11434/v1` | Embedding 服务地址 |
| `rag.chunk_size` | `800` | 分块大小（按 **rune/字符** 计，不是 token） |
| `rag.chunk_overlap` | `120` | 相邻块重叠长度，减轻断句切碎 |
| `rag.top_k` | `5` | 默认检索返回条数（注入 prompt 的上下文块数） |
| `rag.vector_index` | `hnsw` | pgvector 索引：`none` / `ivfflat` / `hnsw` |
| `ocr.min_pdf_text_len` | `40` | PDF 抽字过少时触发 OCR（间接影响扫描件入库质量） |

### A.2 怎么调

**1）分块 `chunk_size` / `chunk_overlap`**

| 场景 | 建议 |
|------|------|
| 短问答、FAQ、说明书条目 | `chunk_size` 400～600，`overlap` 约 10%～15% |
| 长文档、制度/合同 | `chunk_size` 800～1200，`overlap` 100～200 |
| 检索「答非所问、缺上下文」 | 略增大 `chunk_size` 或 `overlap` |
| 噪声多、答不准 | 略减小 `chunk_size`，并检查语料清洗/OCR 质量 |

改分块参数后，需对语料执行 **重新索引**（`POST /api/v1/corpora/{id}/reindex`），旧 chunk 不会自动按新参数重切。

**2）检索条数 `top_k`**

| 现象 | 调整 |
|------|------|
| 回答漏关键信息 | 增大到 8～12（注意 prompt 变长、费用/延迟上升） |
| 回答啰嗦、串题 | 降到 3～5，并提高语料质量 |
| Agent `knowledge_search` | 请求体 `rag.top_k` 可单独覆盖，不必改全局配置 |

**3）Embedding 模型与维度**

- `embed.dimensions` 必须等于模型真实维度（如 `nomic-embed-text` 常用 768）。
- **更换模型或维度** 后必须：改配置 → 迁移/重建 `embedding vector(n)` 列 → 全量 `reindex`。维度不一致会导致写入或检索失败。
- 中英文混排可继续用当前模型；若效果差，优先换更强的 embedding 端点，而不是只加大 `top_k`。

**4）向量索引 `vector_index`**

| 数据规模（约） | 建议 |
|----------------|------|
| &lt; 1 万 chunk | `none` 或 `hnsw` 均可 |
| 1 万～百万 | `hnsw`（默认 `m=16, ef_construction=64`） |
| 极大、内存紧 | 可试 `ivfflat`（需先有数据再建索引，效果依赖 `lists`） |

索引在入库/重建时按配置尝试创建；也可手工 SQL，见 [docs/DB_SCHEMA.md](docs/DB_SCHEMA.md)。

**5）调试流程（推荐）**

1. `POST /api/v1/rag/search` 用同一 `query` 看召回：`score` 为 **余弦距离，越小越相似**。
2. 若相关块排不进 TopK → 查分块/Embedding/语料，而不是先改 LLM。
3. 召回正常但生成差 → 再调 `system_prompt`、模型或 `top_k`。
4. 聊天打开 RAG：`rag.enabled=true` + `corpus_id`；会话也可绑定默认 `corpus_id`。

**6）示例配置片段**

```yaml
embed:
  base_url: "http://localhost:11434/v1"
  model: nomic-embed-text
  dimensions: 768

rag:
  top_k: 5
  chunk_size: 800
  chunk_overlap: 120
  vector_index: hnsw   # none | ivfflat | hnsw
```

