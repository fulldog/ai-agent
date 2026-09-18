# 钉钉机器人 RAG 流式回复

进程内按官方 [Stream 协议](https://open.dingtalk.com/document/development/configure-stream-push) 接收群内 **@机器人** 消息（单聊不需 @），用钉钉 `senderStaffId` 作为会话 `uid`，按 **用户 + 群/单聊 conversationId** 隔离历史。检索本服务知识库后走现有 `CompleteStream`，再把增量推到钉钉 **AI 流式卡片**（钉钉客户端没有 HTTP SSE）。

收消息：本仓库自研 `POST /v1.0/gateway/connections/open` + WebSocket，**不依赖** `dingtalk-stream-sdk-go`。  
发消息：`github.com/alibabacloud-go/dingtalk`（oauth2 / card / robot）；`sessionWebhook` 用本地 HTTP。

## 前置

1. 开放平台应用已发布，机器人已加入目标群。
2. 权限：机器人发消息、`Card.Instance.Write`、`Card.Streaming.Write`。
3. 开发者后台配置 AI 卡片模板，把模板 ID 写入 `dingtalk.card_template_id`。
4. 本服务须启用 PostgreSQL（会话与 RAG 落库）。

## 后台必须开两处 Stream

实现代码前，应用必须：

1. 开发者后台 → 目标应用 → **开发配置 → 事件订阅** → 选 **Stream 模式推送**。
2. 本服务跑起来后点 **验证 Stream 模式通道**，成功再 **保存并触发 Ticket 推送**。
3. **应用能力 → 机器人 → 消息接收模式** 选 **Stream**（与事件订阅是两处开关）。
4. 凭证用「凭证与基础信息」的 **Client ID / Client Secret**，不要填 `robot_code`。

少任一步，`POST /v1.0/gateway/connections/open` 都会回笼统 `systemError`。

## 配置

见 `configs/config.example.yaml` 的 `dingtalk` 段。密钥也可用环境变量：

`DINGTALK_ENABLED`、`DINGTALK_CLIENT_ID`、`DINGTALK_CLIENT_SECRET`、`DINGTALK_CARD_TEMPLATE_ID`。

```yaml
dingtalk:
  enabled: true
  client_id: "your-app-key"
  client_secret: "your-app-secret"
  card_template_id: "your-ai-card-template-id"  # 可选
```

发卡片/群消息时的 robotCode 使用同一应用的 Client ID，无需单独配置。卡片模板未配或创建失败时：整段回复走 `sessionWebhook`；群聊还可再降级到 `robot_1_0.OrgGroupSend`。

## 消息处理

1. 群聊仅处理 `isInAtList=true`；去掉 `@xxx` 得到 query。
2. 先检索语料库：query 命中语料库 **名称** 时只搜这些库，否则在全部库上向量检索。余弦距离（`score`）大于 `rag.max_distance` 的片段视为未命中（默认 `0.55`；设为 `0` 则不过滤）。
3. **无相关命中**时直接回复「语料库未收录相关知识」，不调用大模型。用户明确要求联网查询（如「联网查询」「请联网」「上网搜」「web search」）时除外：去掉这些用语后再检索；仍无命中则走大模型，并对通义开启 `enable_search`（`forced_search`）。
4. `FindOrCreate` 会话：`channel=dingtalk`，`channel_session_id=conversationId`，`uid=senderStaffId`。
5. 有语料命中（或强制联网）时 `CompleteStream`；卡片节流更新，结束 `isFinalize=true`。
6. 第一期仅文字消息。

企业内部群且机器人已上架后才会有 `senderStaffId`；为空时会提示无法识别用户。

## 日志

每条实际处理的钉钉消息共用同一个 `request_id`（钉钉 `msgId`，为空则生成 UUID），同步写入：

1. 文本日志 `logs/info-*.log` 与 `logs/access-*.log`（`step=1..4`）
2. 表 `request_logs`：同一 `request_id` 一行，收到消息时插入，后续步骤更新。`request_body` 为四步 JSON（receive / rag / llm_request / result），`response_preview` 为本次回复。`method=STREAM`，`path=/dingtalk/bot/messages`。

| step | event | 内容 |
|------|--------|------|
| 1 | `dingtalk.receive` | 收到的原文、发送者、群/单聊 |
| 2 | `dingtalk.rag` | 语料检索 query、命中条数、距离、分块内容 |
| 3 | `dingtalk.llm_request` | 发给模型的对话；语料未命中且未强制联网时跳过 |
| 4 | `dingtalk.result` | 本次回复、outcome、耗时、错误 |

调用了大模型时，`llm_call_logs` 与 `logs/llm-*.log` 的 `request_id` 相同。控制台「日志」页可按路径 `/dingtalk/bot/messages` 筛选；点开详情可看 `request_body.steps` 与 `llm_calls`。

## Stream 建连失败（`systemError` / 系统错误）

日志里 `dingtalk stream handshake failed` 或 `dingtalk stream stopped` 且 `code=systemError`，发生在握手接口 `POST /v1.0/gateway/connections/open`，**还没开始收群消息**。进程会自动重试。按下面逐项核对：

1. **`client_id` / `client_secret` 必须是同一应用的一对**  
   开放平台 → 应用 → **凭证与基础信息** 里的 **Client ID（旧称 AppKey）** 和 **Client Secret（旧称 AppSecret）**。  
   **不要把 `robot_code` 填进 `client_id`。**  
   启动日志若出现 `换票失败`，就是这对凭证错了。

2. **事件订阅必须是 Stream，且通道验证成功**  
   开放平台 → **开发配置 → 事件订阅** → **Stream 模式推送**。服务启动后再点「验证 Stream 模式通道」。

3. **机器人「消息接收模式」必须是 Stream**  
   开放平台 → 应用能力 → **机器人** → 配置 → **消息接收模式** 选 **Stream 模式** 并保存。  
   若仍是 HTTP 回调 / 未配置，握手常返回笼统的 `systemError`。

4. **应用已发布、机器人能力已开通**  
   版本状态应为已上架；仅本地创建未发布时，Stream 也可能被拒。

5. **出网**  
   本机需能访问 `https://api.dingtalk.com` 和 `wss://wss-open-connection.dingtalk.com:443`（公司代理/HTTPS 解密会拦 WebSocket）。

换票成功但仍 `systemError`：优先改第 2、3 步（两处 Stream 开关），改完后等约 1 分钟再看重连日志。
