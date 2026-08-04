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

应用启动时由 `internal/database` 执行扩展检查与 AutoMigrate。

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

API Key 第一期可仅存配置文件；若落库可增加 `api_keys`（见文末可选表）。

---

## 3. 表定义

### 3.1 conversations

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| title | TEXT | |
| system_prompt | TEXT | 可空 |
| corpus_id | UUID NULL | 默认 RAG 语料 |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | 软删可选 |

索引：`created_at DESC`。

### 3.2 messages

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| conversation_id | UUID FK → conversations | |
| role | TEXT | `system` / `user` / `assistant` / `tool` |
| content | TEXT | |
| tool_call_id | TEXT NULL | tool 消息关联 |
| tool_calls_json | JSONB NULL | assistant 发起的 tool_calls |
| token_prompt | INT NULL | |
| token_completion | INT NULL | |
| created_at | TIMESTAMPTZ | |

索引：`(conversation_id, created_at)`。

### 3.3 corpora

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| name | TEXT UNIQUE | |
| description | TEXT | |
| embed_model | TEXT | 记录维度对应的模型名 |
| embed_dim | INT | 如 768 |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

### 3.4 documents

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| corpus_id | UUID FK → corpora | |
| title | TEXT | |
| source | TEXT | 文件名或 URI |
| content_hash | TEXT | 去重/变更检测 |
| status | TEXT | `pending` / `indexed` / `failed` |
| error_message | TEXT NULL | |
| created_at | TIMESTAMPTZ | |
| updated_at | TIMESTAMPTZ | |

索引：`(corpus_id, created_at)`。

### 3.5 chunks

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| corpus_id | UUID FK | 冗余便于按库检索 |
| document_id | UUID FK → documents | |
| chunk_index | INT | 文档内序号 |
| content | TEXT | |
| metadata | JSONB | 页码、标题路径等 |
| embedding | vector(dim) | **dim 与 embed 模型一致，全局固定** |
| created_at | TIMESTAMPTZ | |

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

### 3.6 agent_runs

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| conversation_id | UUID NULL | |
| input | TEXT | |
| output | TEXT | 最终回复 |
| model | TEXT | |
| status | TEXT | `running` / `succeeded` / `failed` / `cancelled` |
| max_steps | INT | |
| step_count | INT | |
| error_message | TEXT NULL | |
| prompt_tokens | INT | |
| completion_tokens | INT | |
| created_at | TIMESTAMPTZ | |
| finished_at | TIMESTAMPTZ NULL | |

### 3.7 agent_steps

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| run_id | UUID FK → agent_runs | |
| step_index | INT | |
| kind | TEXT | `llm` / `tool_call` / `tool_result` / `final` |
| tool_name | TEXT NULL | |
| input_json | JSONB NULL | |
| output_text | TEXT NULL | |
| created_at | TIMESTAMPTZ | |

索引：`(run_id, step_index)`。

### 3.8 request_logs

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| request_id | TEXT UNIQUE | 与 Header / zap 关联 |
| api_key_id | TEXT | 密钥标识（非明文） |
| method | TEXT | |
| path | TEXT | |
| path_template | TEXT | 如 `/api/v1/chat/completions/stream` |
| status | INT | |
| latency_ms | INT | |
| request_body | TEXT | 可截断存储 |
| response_preview | TEXT | 流式为拼接预览 |
| stream | BOOLEAN | |
| sse_event_count | INT NULL | 流式事件数 |
| conversation_id | UUID NULL | |
| agent_run_id | UUID NULL | |
| error_message | TEXT NULL | |
| created_at | TIMESTAMPTZ | |

索引：`(created_at DESC)`、`request_id`、`(conversation_id)`、`(agent_run_id)`。

### 3.9 llm_call_logs

| 列 | 类型 | 说明 |
|----|------|------|
| id | UUID PK | |
| request_id | TEXT | 关联 HTTP |
| conversation_id | UUID NULL | |
| agent_run_id | UUID NULL | |
| provider | TEXT | `deepseek` |
| model | TEXT | |
| stream | BOOLEAN | |
| status | TEXT | `ok` / `error` |
| prompt_tokens | INT | |
| completion_tokens | INT | |
| latency_ms | INT | |
| request_summary | TEXT | 消息条数、tools 等摘要，避免存全量密钥 |
| error_message | TEXT NULL | |
| created_at | TIMESTAMPTZ | |

索引：`(created_at DESC)`、`(request_id)`。

### 3.10 token_usage（可选，M5）

按日聚合，便于报表：

| 列 | 类型 | 说明 |
|----|------|------|
| id | BIGSERIAL PK | |
| day | DATE | |
| model | TEXT | |
| prompt_tokens | BIGINT | |
| completion_tokens | BIGINT | |
| request_count | BIGINT | |

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

- UUID：`uuid` 或 `string` + `type:uuid`
- JSONB：`datatypes.JSON` 或 `json.RawMessage`
- `vector(dim)`：可用 `pgvector-go` 类型，或 raw SQL 写入；AutoMigrate 对自定义类型需额外 `CREATE TABLE`/migration
- 建议 M1 对 `chunks` 使用显式 SQL migration，避免 GORM 无法正确生成 `vector(n)`

---

## 6. 与里程碑关系

| 里程碑 | 表 |
|--------|-----|
| M1 | 扩展 vector；request_logs 骨架；连接池 |
| M2 | conversations、messages、llm_call_logs |
| M3 | corpora、documents、chunks + 向量索引 |
| M4 | agent_runs、agent_steps |
| M5 | logs 查询优化、token_usage（可选） |
