# 多厂商 LLM 接入（YAML）

密钥仍放在配置文件 / 环境变量，**不入库**。各厂商走 OpenAI 兼容 `chat/completions`（现有 `llm.Client`）。

## 配置

```yaml
llm:
  default_provider: deepseek
  timeout_seconds: 120
  providers:
    deepseek:
      base_url: "https://api.deepseek.com/v1"
      api_key: ""          # 或环境变量 DEEPSEEK_API_KEY
      default_model: "deepseek-v4-flash"
    qwen:
      base_url: "https://dashscope.aliyuncs.com/compatible-mode/v1"
      api_key: ""          # DASHSCOPE_API_KEY / QWEN_API_KEY
      default_model: "qwen-plus"
    kimi:
      base_url: "https://api.moonshot.cn/v1"
      api_key: ""          # MOONSHOT_API_KEY / KIMI_API_KEY
      default_model: "moonshot-v1-8k"
    doubao:
      base_url: "https://ark.cn-beijing.volces.com/api/v3"
      api_key: ""          # ARK_API_KEY / DOUBAO_API_KEY
      default_model: "ep-xxxx"   # 方舟接入点 ID 或模型名
```

兼容旧写法：仅配置顶层 `llm.provider` / `base_url` / `api_key` / `default_model`，会合并进默认厂商。

也可在 `providers` 下增加自定义 key（任意名字），填兼容网关的 `base_url` + `api_key`。

## 请求

Chat / Agent 增加可选字段：

| 字段 | 说明 |
|------|------|
| `provider` | `deepseek` / `qwen` / `kimi` / `doubao`；别名 `dashscope`→qwen、`moonshot`→kimi、`ark`→doubao |
| `model` | 覆盖该厂商 `default_model` |

```json
{
  "conversation_id": "...",
  "message": "你好",
  "provider": "qwen",
  "model": "qwen-plus"
}
```

未传 `provider` 时用 `llm.default_provider`。

## 列表接口

`GET /api/v1/models`（需 API Key）返回厂商列表，含 `configured`（是否已配 key），**不含密钥**。

## 注意

- Agent Tool Calling 依赖厂商对 `tools` 的支持；不支持时可能无法完成 Agent 循环。
- 豆包 `default_model` 通常为方舟 **接入点 ID**。
- Embedding 仍走 `embed.*`，与 Chat 厂商无关。
