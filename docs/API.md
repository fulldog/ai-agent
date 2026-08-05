# AI Agent API 草案

> Base URL：`http://localhost:18090`  
> API 前缀：`/api/v1`  
> 关联：[PROJECT_PLAN.md](./PROJECT_PLAN.md)、[DB_SCHEMA.md](./DB_SCHEMA.md)  
> **Apifox 导入**：[openapi.yaml](./openapi.yaml)（OpenAPI 3.0）→ Apifox「导入」→ 「OpenAPI」选择该文件

---

## 1. 通用约定

### 1.1 Headers

| Header | 必填 | 说明 |
|--------|------|------|
| `X-API-Key` | 是（业务 API） | 配置中校验；`/health`、`/metrics` 默认不要求 |
| `X-Request-ID` | 否 | 客户端可传入；缺省由服务端生成 UUID |
| `Content-Type` | POST/PUT | `application/json`（上传文档可用 `multipart/form-data`） |

### 1.2 错误响应

```json
{
  "error": {
    "code": "unauthorized",
    "message": "invalid api key"
  },
  "request_id": "uuid"
}
```

常见 HTTP 状态：`400` 参数错误、`401` 鉴权失败、`404` 不存在、`429` 限流（预留）、`500` 内部错误。

### 1.3 SSE 约定

流式接口：`Content-Type: text/event-stream; charset=utf-8`

```
event: delta
data: {"content":"..."}

event: done
data: {"status":"ok"}

event: error
data: {"message":"..."}
```

Agent 额外事件：`tool_call`、`tool_result`（可选 `thought`）。

---

## 2. 系统接口

### GET `/health`

健康检查。无需 API Key。

**响应** `200`

```json
{
  "status": "ok",
  "db": "up"
}
```

### GET `/metrics`

Prometheus 文本格式指标。默认无需 API Key。  
若配置 `metrics.protect: true`，则需 admin 级 API Key。

---

## 3. 会话 Conversations

### POST `/api/v1/conversations`

创建会话。

```json
{
  "title": "optional title",
  "system_prompt": "optional",
  "corpus_id": "optional-uuid-for-default-rag"
}
```

**响应** `201`：会话对象（含 `id`）。

### GET `/api/v1/conversations`

列表。Query：`limit`（默认 20）、`offset`。

### GET `/api/v1/conversations/:id`

详情。

### DELETE `/api/v1/conversations/:id`

删除会话及消息（软删或硬删，实现阶段定）。

### GET `/api/v1/conversations/:id/messages`

消息列表。Query：`limit`、`before_id`（可选游标）。

---

## 4. 聊天 Chat

多厂商说明见 [MULTI_LLM.md](./MULTI_LLM.md)。可选 `provider`（`deepseek` / `qwen` / `kimi` / `doubao`），未传则用配置默认厂商。

### GET `/api/v1/models`

列出 YAML 中配置的厂商（含是否已配 key，**不含密钥**）。

### POST `/api/v1/chat/completions`

同步补全（非流式）。

```json
{
  "conversation_id": "uuid",
  "message": "用户问题",
  "provider": "deepseek",
  "model": "deepseek-v4-flash",
  "rag": {
    "enabled": true,
    "corpus_id": "uuid",
    "top_k": 5
  },
  "temperature": 0.7,
  "max_tokens": 2048
}
```

**响应** `200`

```json
{
  "conversation_id": "uuid",
  "message_id": "uuid",
  "content": "助手回复",
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

### POST `/api/v1/chat/completions/stream`

流式补全。请求体同同步接口。SSE 事件：`delta`、`done`、`error`。

### POST `/api/v1/chat/analyze`

**上传文件直接交给大模型分析，不写入语料库。**  
适合：合同 PDF 抽字段，一次性返回 JSON。

#### 合同抽字段示例（推荐）

```bash
curl -X POST http://localhost:18090/api/v1/chat/analyze \
  -H "X-API-Key: change-me-api-key" \
  -F "file=@/path/to/contract.pdf" \
  -F "provider=deepseek" \
  -F 'fields=["合同名称","合同编号","合同联系人","合同有效期"]'
```

也可用逗号分隔：`-F "fields=合同名称,合同编号,合同联系人,合同有效期"`

**成功响应（关注 `data`）**：

```json
{
  "data": {
    "合同名称": "XX技术服务合同",
    "合同编号": "HT-2026-001",
    "合同联系人": "张三",
    "合同有效期": "2026-01-01 至 2026-12-31"
  },
  "file_name": "contract.pdf",
  "file_chars": 12345,
  "truncated": false,
  "content": "{...}",
  "usage": { "prompt_tokens": 1000, "completion_tokens": 80, "total_tokens": 1080 }
}
```

找不到的字段值为 `null`。业务侧直接用 `data` 即可。

#### 自定义问题 + 强制 JSON

```bash
curl -X POST http://localhost:18090/api/v1/chat/analyze \
  -H "X-API-Key: change-me-api-key" \
  -F "file=@/path/to/contract.pdf" \
  -F "response_format=json" \
  -F "message=根据文件，请帮我分析出：合同名称、合同编号、合同联系人、合同有效期，并以 JSON 对象返回"
```

#### 表单字段

| 字段 | 必填 | 说明 |
|------|------|------|
| `file` | 是 | pdf/docx/txt/图片等 |
| `fields` | 与 message 二选一 | 抽取字段列表（JSON 数组或逗号分隔）；有则自动要求 JSON |
| `message` | 与 fields 二选一 | 自定义问题 |
| `force_reread` | 否 | `true`：强制重新识别；**成功后**才软删旧缓存 |
| `provider` / `model` | 否 | 对话厂商与模型。`qwen`/`kimi` 走云端传文件；`deepseek` 等走本机读文件 |
| `conversation_id` | 否 | 有则写入会话；无则一次性 |

成功响应额外字段：`cache_hit`、`content_hash`、`extraction_id`、`extract_backend`（multipart 上传时）。

**识别方式（由 `provider` 决定，不再使用 extract_backend）：**

| provider | 行为 |
|----------|------|
| `qwen` | 上传到通义 → 用 `file_id` 直接对话（默认 `qwen-long`）→ **异步**拉正文写库 |
| `kimi` | 上传到 Kimi → 拉正文 → 对话 + 落库 |
| `deepseek` 等 | 本机 OCR/解析 → 正文塞进 prompt 对话 |

同一文件强刷/抽取时若写锁被占用，返回 HTTP 409：`文档正在识别中`（进程内读写锁）。

同一文件（内容 SHA256 相同）再次上传且未传 `force_reread` 时，直接使用上次抽取文本，跳过 OCR。原始文件与抽取 txt 保存在 `attachments/YYYY/MM/DD/`（见 [DB_SCHEMA.md](./DB_SCHEMA.md) `file_extractions`）。

JSON 请求体也可用：`content`（正文）+ `fields` / `message`（不走文件缓存）。

过长正文默认截断约 8 万字（`truncated: true`）。扫描版 PDF 需本机 OCR。

### POST `/api/v1/chat/analyze/stream`

同上，SSE：`delta` / `done` / `error`。结构化结果在 `done` 的 `data` 里；抽字段场景更推荐同步接口。

---

## 5. Agent

工具名、中文描述约定、**如何新增 Tool** 见 [AGENT_TOOLS.md](./AGENT_TOOLS.md)。

### POST `/api/v1/agent/runs`

同步 Agent 运行（适合短任务）。

```json
{
  "conversation_id": "uuid",
  "input": "帮我查知识库里关于 SLA 的说明",
  "provider": "deepseek",
  "model": "deepseek-v4-flash",
  "max_steps": 8,
  "tools": ["knowledge_search", "current_time", "calculator"],
  "rag": {
    "corpus_id": "uuid",
    "top_k": 5
  }
}
```

**响应** `200`：最终文本、`run_id`、`steps` 摘要、`usage`。

### POST `/api/v1/agent/runs/stream`

流式 Agent。请求体同上。SSE：`tool_call`、`tool_result`、`delta`、`done`、`error`。

示例：

```
event: tool_call
data: {"name":"knowledge_search","arguments":{"query":"SLA"}}

event: tool_result
data: {"name":"knowledge_search","content":"..."}

event: delta
data: {"content":"根据知识库..."}

event: done
data: {"status":"ok","run_id":"uuid"}
```

### GET `/api/v1/agent/runs/:id`

查询某次 Agent 运行及步骤（调试/审计）。

---

## 6. 语料 Corpus

### POST `/api/v1/corpora`

```json
{
  "name": "product-docs",
  "description": "产品文档"
}
```

### GET `/api/v1/corpora`

列表。

### GET `/api/v1/corpora/:id`

详情（含文档数、chunk 数等统计，可选）。

### DELETE `/api/v1/corpora/:id`

删除语料及下属文档/chunk。

### POST `/api/v1/corpora/:id/documents`

上传文档。`multipart/form-data`：`file`，可选 `force_reread`（强制重新抽取并软删旧缓存）。支持：

- 文本：txt/md/…
- Word：docx
- PDF：文字层提取；扫描件 OCR
- 图片：png/jpg/…（OCR）

或 JSON：

```json
{
  "title": "sla.md",
  "content": "全文..."
}
```

multipart 响应含 `document`、`cache_hit`、`content_hash`、`extraction_id`。附件缓存见 [EXTRACT.md](./EXTRACT.md)、[DB_SCHEMA.md](./DB_SCHEMA.md) `file_extractions`。

服务端：解析/OCR（或命中缓存）→ 分块 → Embedding → 写入 `chunks`。

### GET `/api/v1/corpora/:id/documents`

文档列表。

### DELETE `/api/v1/corpora/:id/documents/:doc_id`

删除文档及其 chunks。

### POST `/api/v1/corpora/:id/reindex`

重建索引（重新 Embedding；可选重建 pgvector 索引）。

---

## 7. RAG 调试

### POST `/api/v1/rag/search`

```json
{
  "corpus_id": "uuid",
  "query": "SLA 响应时间",
  "top_k": 5
}
```

**响应** `200`

```json
{
  "results": [
    {
      "chunk_id": "uuid",
      "document_id": "uuid",
      "content": "...",
      "score": 0.12,
      "metadata": {}
    }
  ]
}
```

`score` 为 pgvector 距离（越小越相似，实现阶段在文档中注明）。

---

## 8. 请求日志

### GET `/api/v1/logs/requests`

查询请求日志（需 API Key；可选仅 admin key）。

Query：`limit`、`offset`、`request_id`、`conversation_id`、`agent_run_id`、`from`、`to`、`path`。

**响应**：`request_logs` 列表（含摘要字段，大字段可截断）。

### GET `/api/v1/logs/requests/:id`

单条详情（含更完整 body 预览）。

---

## 9. 接口一览

| 模块 | 方法 | 路径 | 鉴权 |
|------|------|------|------|
| Health | GET | `/health` | 否 |
| Metrics | GET | `/metrics` | 默认否 |
| Conversations | CRUD | `/api/v1/conversations` | 是 |
| Messages | GET | `/api/v1/conversations/:id/messages` | 是 |
| Models | GET | `/api/v1/models` | 是 |
| Chat | POST | `/api/v1/chat/completions` | 是 |
| Chat Stream | POST | `/api/v1/chat/completions/stream` | 是 |
| Chat Analyze | POST | `/api/v1/chat/analyze` | 是 |
| Chat Analyze Stream | POST | `/api/v1/chat/analyze/stream` | 是 |
| Agent | POST | `/api/v1/agent/runs` | 是 |
| Agent Stream | POST | `/api/v1/agent/runs/stream` | 是 |
| Agent Run | GET | `/api/v1/agent/runs/:id` | 是 |
| Corpus | CRUD + upload | `/api/v1/corpora`、`.../documents` | 是 |
| Reindex | POST | `/api/v1/corpora/:id/reindex` | 是 |
| RAG | POST | `/api/v1/rag/search` | 是 |
| Logs | GET | `/api/v1/logs/requests` | 是 |

---

## 10. 里程碑对应

| 里程碑 | 优先实现的接口 |
|--------|----------------|
| M1 | `/health`、`/metrics`、鉴权中间件骨架 |
| M2 | Conversations、Chat（含 stream） |
| M3 | Corpus、RAG search、Chat 可选 RAG |
| M4 | Agent runs（含 stream） |
| M5 | Logs 查询、用量相关扩展 |
