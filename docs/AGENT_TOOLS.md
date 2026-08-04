# Agent Tools 规范

代码位置：`internal/service/agent/tools`  
运行循环：`internal/service/agent/service.go`（经 `Registry` 选工具并执行）

本文约定如何**新增 / 修改** Agent 可调用工具，保证与 DeepSeek Tool Calling、配置与文档一致。

---

## 1. 架构

```text
请求 tools[] / agent.default_tools
        │
        ▼
 agent.Service.Run
        │
        ├─ Registry.Specs(names)  → 发给 LLM 的 llm.ToolSpec
        └─ Registry.Exec(name, args, Env) → Tool.Exec → 字符串结果回填 role=tool
```

| 类型 | 职责 |
|------|------|
| `Tool` | 单个工具：名称、Schema、执行逻辑 |
| `Registry` | 按 `Name()` 注册；按请求名列表过滤 Specs；按名 Exec |
| `Env` | 执行期注入（语料 ID、TopK、RAG 等），避免工具绑死 `*agent.Service` |

---

## 2. 命名与描述约定（必须遵守）

| 项 | 规范 |
|----|------|
| 工具名 `Name()` | 英文 **snake_case**，与 Spec 中 `function.name`、配置项、请求 `tools[]` **完全一致** |
| `Description` | **中文**，说明何时调用、做什么 |
| 参数 `properties.*.description` | **中文** |
| JSON Schema | 写清 `type` / `required`；枚举用 `enum` |
| `Exec` 成功返回值 | **字符串**（模型可读；复杂结构可 JSON 序列化成 string） |
| `Exec` 失败 | 返回 `error`；上层会把 `err.Error()` 当 tool 内容回传（**错误文案可用中文**） |
| 副作用 | 写库、外呼等须在 Description 中写明；默认内置工具以只读为主 |
| 超时 / 取消 | 外调服务必须尊重 `ctx`；长耗时可 `context.WithTimeout` |
| 安全 | 勿默认暴露任意 URL 抓取、任意命令执行；若必须，需白名单与鉴权设计 |

错误示例：把工具名写成中文、Description 只写英文、参数无 `required` 导致模型漏传。

---

## 3. 接口定义

```go
type Tool interface {
	Name() string
	Spec() llm.ToolSpec
	Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error)
}

type Env struct {
	CorpusID    *uuid.UUID
	TopK        int
	DefaultTopK int
	RAG         *rag.Service
	// 新依赖优先挂到 Env，或由具体 Tool 结构体持有只读配置
}
```

- `Spec()` 必须返回 OpenAI 兼容的 `type: "function"` 结构（本项目 `llm.ToolSpec`）。
- `args` 为模型给出的 JSON；解析失败应返回明确中文错误。

---

## 4. 新增工具清单（Checklist）

1. **实现**  
   在 `internal/service/agent/tools/` 新增文件（如 `weather.go`），实现 `Tool`：
   - `Name()` / `Spec()`（中文描述）/ `Exec(...)`
2. **注册**  
   在 `registry.go` 的 `Default()` 中加入该工具实例。
3. **默认启用（可选）**  
   - `configs/config.example.yaml` → `agent.default_tools`  
   - `internal/config/config.go` 中 `Agent.DefaultTools` 默认值（如需与 example 一致）  
   - 本地 `configs/config.yaml`（勿提交密钥）
4. **文档与契约**  
   - 更新本文「内置工具」表  
   - `docs/API.md` Agent 示例中的 `tools`  
   - `docs/openapi.yaml` 中 `tools` 枚举 / example  
   - `README.md` 技术栈里工具列表（如有）
5. **测试**  
   为纯逻辑工具补充 `*_test.go`（解析参数、边界、除零等）。
6. **联调**  
   `POST /api/v1/agent/runs`，请求体 `"tools": ["your_tool"]`，确认出现 `tool_call` / `tool_result`。

未在 `Default()` 注册的工具，即使写了代码也不会出现在 Specs 中。  
已注册但未列入 `default_tools` / 请求 `tools` 的工具，**不会**暴露给当次运行的模型。

---

## 5. 最小代码模板

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

type ExampleTool struct{}

func (ExampleTool) Name() string { return "example_tool" }

func (ExampleTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "example_tool",
			Description: "用一句话说明此工具的用途与调用时机",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "查询内容",
					},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (ExampleTool) Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("参数无效: %w", err)
	}
	if p.Query == "" {
		return "", fmt.Errorf("query 不能为空")
	}
	_ = ctx
	_ = env
	return "结果文本", nil
}
```

注册：

```go
func Default() *Registry {
	return NewRegistry(
		KnowledgeSearch{},
		CurrentTime{},
		Calculator{},
		ExampleTool{}, // 新增
	)
}
```

配置：

```yaml
agent:
  max_steps: 8
  default_tools:
    - knowledge_search
    - current_time
    - calculator
    - example_tool   # 需要默认开启时再加
```

请求覆盖：

```json
{
  "input": "……",
  "tools": ["example_tool", "current_time"]
}
```

---

## 6. 内置工具

| 名称 | 说明 | 依赖 Env / 请求 |
|------|------|-----------------|
| `knowledge_search` | 在知识库（RAG）中检索相关文本片段 | 需要 `rag.corpus_id`；`top_k` 可选 |
| `current_time` | 获取服务器当前时间（RFC3339） | 无 |
| `calculator` | 简单四则运算 `a op b`（`+ - * /`） | 无 |

需要 RAG 时请求示例：

```json
{
  "input": "查一下 SLA 怎么写的",
  "tools": ["knowledge_search"],
  "rag": { "corpus_id": "<uuid>", "top_k": 5 }
}
```

---

## 7. 配置与 API

| 来源 | 字段 | 说明 |
|------|------|------|
| 配置文件 | `agent.default_tools` | 请求未传 `tools` 时使用 |
| 配置文件 | `agent.max_steps` | 工具循环最大步数 |
| 请求体 | `tools` | 覆盖默认列表；只传需要的名字 |
| 请求体 | `rag.corpus_id` / `rag.top_k` | 供 `knowledge_search` |

HTTP 接口详见 [API.md](./API.md) § Agent；契约见 [openapi.yaml](./openapi.yaml)。

---

## 8. 依赖扩展说明

若新工具需要新下游（HTTP Client、第三方 SDK 等）：

1. 优先在 `Env` 增加字段，由 `agent.Service.Run` 组装时注入；或  
2. 工具结构体持有依赖（在 `Default()` / `NewRegistry` 时构造），适合无请求态的只读配置。

不要在 `Exec` 里全局 `init` 连库；保持可测试、可替换。

---

## 9. 相关文件索引

| 路径 | 内容 |
|------|------|
| `internal/service/agent/tools/tool.go` | `Tool` / `Env` |
| `internal/service/agent/tools/registry.go` | 注册与 `Default()` |
| `internal/service/agent/tools/*.go` | 各工具实现 |
| `internal/service/agent/service.go` | Agent 循环调用 Registry |
| `configs/config.example.yaml` | `agent.default_tools` |
| `docs/API.md` / `docs/openapi.yaml` | 对外契约 |
