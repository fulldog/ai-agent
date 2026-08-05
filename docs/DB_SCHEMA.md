# AI Agent 数据库表结构草案

> 引擎：PostgreSQL 16+（Community，免费）  
> 扩展：pgvector  
> ORM：GORM  
> 关联：[PROJECT_PLAN.md](./PROJECT_PLAN.md)、[API.md](./API.md)、**安装配置 → [POSTGRES.md](./POSTGRES.md)**

---

## 1. 初始化

安装 PostgreSQL、编译 pgvector、建库建用户、配置 `DATABASE_URL` 的完整步骤见 **[POSTGRES.md](./POSTGRES.md)**。

```sql
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- 可选，用于 gen_random_uuid()
```

应用启动时由 `internal/database` 执行扩展检查、AutoMigrate，并写入 **PostgreSQL 表/列 COMMENT**（来源：GORM 字段 `comment` 标签 + `EnsureSchemaComments`）。

在 psql 中可查看：

```sql
\d+ conversations
SELECT cols.column_name, pgd.description
FROM pg_catalog.pg_statio_all_tables AS st
JOIN pg_catalog.pg_description pgd ON pgd.objoid = st.relid
JOIN information_schema.columns cols
  ON cols.table_schema = st.schemaname AND cols.table_name = st.relname
 AND cols.ordinal_position = pgd.objsubid
WHERE st.relname = 'conversations';
```

**配置示例 DSN**

```text
postgres://ai_agent:password@127.0.0.1:5432/ai_agent?sslmode=disable
```

环境变量：`DATABASE_URL` 或 `PG_DSN`（优先级高于配置文件）。

---

## 2. 表一览

| 表名 | 用途 |
|------|------|
| `conversations` | 会话 |
| `messages` | 消息 |
| `corpora` | 语料库 |
| `documents` | 语料文档 |
| `chunks` | 分块 + embedding（pgvector） |
| `agent_runs` | Agent 运行 |
| `agent_steps` | Agent 步骤 |
| `request_logs` | HTTP 请求完整日志 |
| `llm_call_logs` | 上游 LLM 调用日志 |
| `token_usage` | 可选按日聚合（M5） |
| `file_extractions` | 上传文件与抽取文本关联（内容哈希缓存） |

API Key 第一期可仅存配置文件；若落库可增加 `api_keys`（见文末可选表）。

---

## 3. 表定义

> 下列「说明」与库内 `COMMENT ON COLUMN` / GORM `comment` 标签一致。

### 3.1 conversations（会话表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 会话ID |
| title | TEXT | 会话标题 |
| system_prompt | TEXT | 系统提示词（可空） |
| corpus_id | UUID NULL | 默认绑定的语料库ID（RAG） |
| created_at | TIMESTAMPTZ | 创建时间 |
| updated_at | TIMESTAMPTZ | 更新时间 |
| deleted_at | TIMESTAMPTZ NULL | 软删除时间 |

索引：`deleted_at`；建议按 `created_at DESC` 查询。

### 3.2 messages（会话消息表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 消息ID |
| conversation_id | UUID FK → conversations | 所属会话ID |
| role | TEXT | 角色：`system` / `user` / `assistant` / `tool` |
| content | TEXT | 消息正文 |
| tool_call_id | TEXT NULL | tool 消息关联的 tool_call_id |
| tool_calls_json | JSONB NULL | assistant 发起的 tool_calls JSON |
| token_prompt | INT NULL | 本条消耗的 prompt token |
| token_completion | INT NULL | 本条消耗的 completion token |
| created_at | TIMESTAMPTZ | 创建时间 |

索引：`(conversation_id, created_at)`。

### 3.3 corpora（语料库表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 语料库ID |
| name | TEXT UNIQUE | 语料库名称（唯一） |
| description | TEXT | 语料库描述 |
| embed_model | TEXT | Embedding 模型名 |
| embed_dim | INT | 向量维度（如 768） |
| created_at | TIMESTAMPTZ | 创建时间 |
| updated_at | TIMESTAMPTZ | 更新时间 |

### 3.4 documents（语料文档表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 文档ID |
| corpus_id | UUID FK → corpora | 所属语料库ID |
| title | TEXT | 文档标题 |
| source | TEXT | 来源（文件名 / URI 等） |
| content_hash | TEXT | 内容哈希（去重 / 变更检测） |
| status | TEXT | 状态：`pending` / `indexing` / `ready` / `failed` |
| error_message | TEXT NULL | 索引失败原因 |
| created_at | TIMESTAMPTZ | 创建时间 |
| updated_at | TIMESTAMPTZ | 更新时间 |

索引：`(corpus_id, created_at)`、`status`。

### 3.4b file_extractions（文件抽取关联表）

上传文件按**内容 SHA256**缓存抽取结果；命中则跳过 OCR/解析。强制重读时将旧行 `is_deleted=1`，新建一行并保留旧原始文件与 txt。

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 抽取记录ID |
| content_hash | TEXT | 原始文件 SHA256 |
| original_name | TEXT | 上传文件名 |
| ext | TEXT | 扩展名 |
| size_bytes | BIGINT | 原始字节数 |
| source_path | TEXT | 原始文件相对路径（如 `attachments/2026/08/05/{id}_source.pdf`） |
| text_path | TEXT | 抽取文本相对路径（可空；无正文则不落盘） |
| text_chars | INT | 抽取字符数 |
| extract_backend | TEXT | `local` / `kimi` / `qwen` |
| remote_file_id | TEXT | 云端文件 ID（可空） |
| status | TEXT | `ready` / `failed` |
| error_message | TEXT | 失败原因 |
| is_deleted | SMALLINT 默认 0 | 软删除：`0` 有效，`1` 已删 |
| created_at | TIMESTAMPTZ | 创建时间 |
| updated_at | TIMESTAMPTZ | 更新时间（软删时刷新） |

索引：`(content_hash, is_deleted)`。查询当前缓存：`content_hash = ? AND is_deleted = 0 AND status = 'ready'`。

落盘根目录配置：`storage.attachments_dir`（默认 `attachments`），子目录按 `YYYY/MM/DD`。

抽取后端见配置 `extract.backend`：`local`（本机 OCR）/ `kimi` / `qwen`（云端 Files）。**有正文才写 txt**；`text_path` 空或文件丢失时下次命中强制重抽（优先读 `source_path`）。

### 3.5 chunks（文档分块与向量表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 分块ID |
| corpus_id | UUID FK | 所属语料库ID（冗余便于按库检索） |
| document_id | UUID FK → documents | 所属文档ID |
| chunk_index | INT | 文档内分块序号（从 0 起） |
| content | TEXT | 分块文本内容 |
| metadata | JSONB | 分块元数据 JSON（页码、标题路径等） |
| embedding | vector(dim) | 分块向量（pgvector）；**dim 与 embed.dimensions 一致** |
| created_at | TIMESTAMPTZ | 创建时间 |

```sql
-- 示例：dim = 768（需与 configs.embed.dimensions 一致）
-- embedding vector(768) NOT NULL

CREATE INDEX IF NOT EXISTS idx_chunks_corpus ON chunks (corpus_id);

-- 数据量较小时可先不建向量索引；扩大后：
-- IVFFlat（需先分析数据）
-- CREATE INDEX idx_chunks_embedding_ivfflat ON chunks
--   USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- HNSW（推荐作为默认生产索引）
-- CREATE INDEX idx_chunks_embedding_hnsw ON chunks
--   USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);
```

**检索示例**

```sql
SELECT id, document_id, content, embedding <=> $1::vector AS distance
FROM chunks
WHERE corpus_id = $2
ORDER BY embedding <=> $1::vector
LIMIT $3;
```

距离越小越相似（cosine distance）。应用层可转换为相似度 `1 - distance` 展示。

> 注意：PostgreSQL 一列 `vector(n)` 的 `n` 在建表时固定。更换 Embedding 模型维度需迁移（重建列/表）。

### 3.6 agent_runs（Agent 运行记录表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 运行ID |
| conversation_id | UUID NULL | 关联会话ID（可选） |
| input | TEXT | 用户输入 |
| output | TEXT | 最终输出 |
| model | TEXT | 使用的模型名 |
| status | TEXT | 状态：`running` / `succeeded` / `failed` |
| max_steps | INT | 最大步数上限 |
| step_count | INT | 实际步数 |
| error_message | TEXT NULL | 失败错误信息 |
| prompt_tokens | INT | 累计 prompt token |
| completion_tokens | INT | 累计 completion token |
| created_at | TIMESTAMPTZ | 创建时间 |
| finished_at | TIMESTAMPTZ NULL | 结束时间 |

### 3.7 agent_steps（Agent 步骤明细表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 步骤ID |
| run_id | UUID FK → agent_runs | 所属运行ID |
| step_index | INT | 步骤序号 |
| kind | TEXT | 类型：`llm` / `tool_result` |
| tool_name | TEXT NULL | 工具名（工具步骤时） |
| input_json | JSONB NULL | 步骤输入 JSON |
| output_text | TEXT NULL | 步骤输出文本 |
| created_at | TIMESTAMPTZ | 创建时间 |

索引：`(run_id, step_index)`。

### 3.8 request_logs（HTTP 请求日志表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 日志ID |
| request_id | TEXT UNIQUE | 请求追踪ID（与 Header / zap 关联） |
| api_key_id | TEXT | 调用方 API Key 标识（脱敏，非明文） |
| method | TEXT | HTTP 方法 |
| path | TEXT | 请求路径 |
| path_template | TEXT | 路由模板（如 `/api/v1/chat/completions/stream`） |
| status | INT | HTTP 状态码 |
| latency_ms | INT | 耗时毫秒 |
| request_body | TEXT | 请求体摘要（可截断） |
| response_preview | TEXT | 响应预览（流式为拼接预览） |
| stream | BOOLEAN | 是否 SSE 流式 |
| sse_event_count | INT | SSE 事件数 |
| conversation_id | UUID NULL | 关联会话ID |
| agent_run_id | UUID NULL | 关联 Agent 运行ID |
| error_message | TEXT NULL | 错误信息 |
| created_at | TIMESTAMPTZ | 创建时间 |

索引：`(created_at DESC)`、`request_id`、`(conversation_id)`、`(agent_run_id)`。

### 3.9 llm_call_logs（上游 LLM 调用日志表）

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | 调用日志ID |
| request_id | TEXT | 关联 HTTP 请求ID |
| conversation_id | UUID NULL | 关联会话ID |
| agent_run_id | UUID NULL | 关联 Agent 运行ID |
| provider | TEXT | 厂商：`deepseek` / `qwen` / `kimi` / `doubao` 等 |
| model | TEXT | 模型名 |
| stream | BOOLEAN | 是否流式 |
| status | TEXT | 状态：`ok` / `error` |
| prompt_tokens | INT | prompt token 数 |
| completion_tokens | INT | completion token 数 |
| latency_ms | INT | 耗时毫秒 |
| request_summary | TEXT | 请求摘要（消息条数、tools 等，不存完整正文） |
| error_message | TEXT NULL | 错误信息 |
| created_at | TIMESTAMPTZ | 创建时间 |

索引：`(created_at DESC)`、`(request_id)`。

完整 prompt / 模型回复不入库，写在文本日志 `logs/llm-YYYY-MM-DD.log`（JSON 字段 `messages`、`response`）。关联方式：日志字段 `llm_call_id` = 本表 `id`，另含 `request_id`。

### 3.10 token_usage（可选，M5）

按日聚合，便于报表：

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGSERIAL PK | 主键 |
| day | DATE | 统计日 |
| model | TEXT | 模型名 |
| prompt_tokens | BIGINT | 当日 prompt token 合计 |
| completion_tokens | BIGINT | 当日 completion token 合计 |
| request_count | BIGINT | 当日调用次数 |

唯一约束：`(day, model)`。也可第一期直接从 `llm_call_logs` 聚合，本表延后。

---

## 4. 可选：api_keys 表

若不想只靠配置文件：

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| key_hash | TEXT UNIQUE | 仅存哈希 |
| name | TEXT | |
| role | TEXT | `default` / `admin` |
| enabled | BOOLEAN | |
| created_at | TIMESTAMPTZ | |

第一期默认：**配置文件明文列表 + 环境变量**，表可选。

---

## 5. GORM / 类型注意

- 字段注释：模型使用 `gorm:"comment:中文说明"`；启动 AutoMigrate 会同步到 PostgreSQL 列 COMMENT；表注释与 `chunks.embedding` 由 `EnsureSchemaComments` 补充
- UUID：`uuid` 或 `string` + `type:uuid`
- JSONB：`datatypes.JSON` 或 `json.RawMessage` / `string` + `type:jsonb`
- `vector(dim)`：raw SQL 维护；AutoMigrate 无法可靠管理该列

---

## 6. 与里程碑关系

| 里程碑 | 表 |
|--------|-----|
| M1 | 扩展 vector；request_logs 骨架；连接池 |
| M2 | conversations、messages、llm_call_logs |
| M3 | corpora、documents、chunks + 向量索引 |
| M4 | agent_runs、agent_steps |
| M5 | logs 查询优化、token_usage（可选） |
