# MCP：Calculator

将 Agent 内置的 `calculator` 工具以 **MCP Server**（stdio）再暴露一份，供 Cursor、Claude Desktop 等 MCP 客户端直接调用。运算逻辑与 `internal/service/agent/tools.Calc` 共用。

| 形态 | 入口 | 说明 |
|------|------|------|
| Agent Tool Calling | `tools.Calculator` | `POST /api/v1/agent/runs` 内置循环 |
| **MCP Server** | `cmd/mcp-calculator` | stdio，工具名仍为 `calculator` |

---

## 1. 构建

```bash
cd /path/to/ai-agent
go build -o bin/mcp-demo ./cmd/mcp-demo
```

Windows：

```powershell
go build -o bin\mcp-calculator.exe .\cmd\mcp-calculator
```

---

## 2. 接入 Cursor

在 Cursor MCP 设置中增加（路径改成你的绝对路径）：

```json
{
  "mcpServers": {
    "ai-agent-calculator": {
      "command": "C:\\webapp\\go-app\\ai-agent\\bin\\mcp-demo.exe",
      "args": []
    }
  }
}
```

Linux / macOS：

```json
{
  "mcpServers": {
    "ai-agent-calculator": {
      "command": "/opt/ai-agent/bin/mcp-demo",
      "args": []
    }
  }
}
```

也可用 `go run`（开发用，较慢）：

```json
{
  "mcpServers": {
    "ai-agent-calculator": {
      "command": "go",
      "args": ["run", "./cmd/mcp-demo"],
      "cwd": "C:\\webapp\\go-app\\ai-agent"
    }
  }
}
```

重启 / 重载 MCP 后，应能看到工具 **`calculator`**。

---

## 3. 工具契约

与内置 Agent 工具一致：

| 参数 | 类型 | 说明 |
|------|------|------|
| `a` | number | 左操作数 |
| `b` | number | 右操作数 |
| `op` | string | `+` / `-` / `*` / `/` |

成功返回结构化字段 `result`（十进制字符串）。除零或不支持的 `op` 返回 MCP 工具错误。

---

## 4. 本地冒烟（可选）

无 MCP 客户端时，可用官方 SDK 客户端或任意 MCP inspector 连 stdio。逻辑单测仍走：

```bash
go test ./internal/service/agent/tools/ -run Calculator
```

---

## 5. 相关代码

| 路径 | 内容 |
|------|------|
| `cmd/mcp-calculator/main.go` | MCP Server 入口 |
| `internal/service/agent/tools/calculator.go` | `Calc` + Agent `Calculator` Tool |
| `docs/AGENT_TOOLS.md` | Agent 侧工具规范 |
