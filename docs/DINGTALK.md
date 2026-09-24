# 钉钉机器人 RAG 流式回复

进程内按官方 [Stream 协议](https://open.dingtalk.com/document/development/configure-stream-push) 接收群内 **@机器人** 消息（单聊不需 @），用钉钉 `senderStaffId` 作为会话 `uid`，按 **用户 + 群/单聊 conversationId** 隔离历史。检索本服务知识库后，默认走现有 `CompleteStream`；也可配置 `dingtalk.reply_mode: agent` 走钉钉预检索后的 `Agent.Run`，或 `dingtalk.reply_mode: web` 走与控制台 Agent 页一致的薄包装。增量推到钉钉 **AI 流式卡片**（钉钉客户端没有 HTTP SSE）。

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

`DINGTALK_ENABLED`、`DINGTALK_CLIENT_ID`、`DINGTALK_CLIENT_SECRET`、`DINGTALK_CARD_TEMPLATE_ID`、`DINGTALK_REPLY_MODE`。

```yaml
dingtalk:
  enabled: true
  client_id: "your-app-key"
  client_secret: "your-app-secret"
  card_template_id: "your-ai-card-template-id"  # 可选
  reply_mode: chat  # chat（默认，CompleteStream）| agent（钉钉预检索 + Agent.Run）| web（同控制台 Agent 薄包装）
```

发卡片/群消息时的 robotCode 使用同一应用的 Client ID，无需单独配置。卡片模板未配或创建失败时：整段回复走 `sessionWebhook`；群聊还可再降级到 `robot_1_0.OrgGroupSend`。

## 消息处理

主路径（@ 机器人 / 单聊）顺序与下面三步一致：

1. **先落会话**：按 `senderStaffId` + 钉钉 `conversationId` 取最新未删除内部会话；没有则新建，并立刻写/更新 `request_logs`（带 `conversation_id`）。已软删不会被复用。
2. **本轮 RAG**：群有绑定语料则只在绑定库检索，未绑定才全量；名称命中可再收窄。距离大于 `rag.max_distance` 视为未命中。  
   **早退**（仅 `chat` / `agent`）：无相关命中 **且** 未要求联网 **且** 该会话无历史消息 → 回复「语料库未收录相关知识」，不调 LLM。  
   用户明确要求联网（如「联网查询」）时去掉用语后再检索；仍无命中则继续走 LLM，并对通义开 `enable_search`。`web` 模式跳过该闸门。
3. **LLM + 工具**：带上第 2 步 RAG 结果与会话近期 messages，按 `reply_mode` 作答（`chat` / `agent` / `web`）。Agent 有工具时首轮强制 function call；推诿「未启用工具」会拦截重试。消息与 request_log 照常落库。

补充：

- 群聊仅处理 `isInAtList=true` 或 `atUsers` 非空；去掉 `@xxx` 得到 query。任意入站会 upsert `dingtalk_chats`。
- 打开控制台「钉钉群」或 Bot 启动时，会从已有 `conversations` 回填尚未建档的会话。
- `chat`：预检索 hits 注入 system；`agent`：预注入 hits；`web`：不预注入 hits，由工具循环检索。卡片流式更新；模板未配则走 `sessionWebhook`。
- 第一期仅文字消息。企业内部群且机器人已上架后才有 `senderStaffId`。

## 日志

**凡经 Stream 入站的钉钉消息都会写请求日志**（含未 @ 跳过），共用同一个 `request_id`（钉钉 `msgId`，为空则生成 UUID），同步写入：

1. 文本日志 `logs/info-*.log` 与 `logs/access-*.log`（`step=1..4`；跳过仅有 receive + result）
2. 表 `request_logs`：同一 `request_id` 一行。**所有 @ 机器人消息（及单聊）在进入异步处理前即插入**，后续步骤更新；未 @ 的群消息走 skip 也会插入。`request_body` 为 pipeline JSON（含顶层 `ding_conversation_id` / `conversation_title` / `group` / `conversation_id`），`response_preview` 为本次回复。`method=STREAM`，`path=/dingtalk/bot/messages`。
3. 表 `dingtalk_chats`：任意入站（含未 @）都会按 `conversationId` upsert 群/单聊档案。

重复投递同一 `msgId`：若已有 `request_logs` 行则不覆盖；若首次落库失败则补记一行。

| step | event | 内容 |
|------|--------|------|
| 1 | `dingtalk.receive` | 收到的原文、发送者、群/单聊、群名 `conversation_title`、`ding_conversation_id`、`is_in_at_list` |
| 2 | `dingtalk.rag` | 语料检索 query、命中条数、距离、分块内容（跳过路径无此步） |
| 3 | `dingtalk.llm_request` / `dingtalk.agent` / `dingtalk.web_agent` | chat：发给模型的对话；agent/web：`run_id`、工具调用与入参（含 SQL）、工具返回、LLM 步骤摘要 |
| 4 | `dingtalk.result` | 本次回复、outcome（含 `skipped` / `corpus_miss`）、耗时、错误、群信息 |

`conversation_id`（本服务会话 UUID）在校验通过并 `FindOrCreate` 后尽早写入，语料未命中等早退也会带上。Agent / web 模式还会写入 `agent_run_id`。

`reply_mode: agent` 或 `web` 时，工具明细同时落在 `agent_steps`（控制台「Agent 历史」按 `run_id` 查看），并写入本条请求日志的 steps。

调用了大模型时，`llm_call_logs` 与 `logs/llm-*.log` 的 `request_id` 相同。控制台「日志」页可按路径 `/dingtalk/bot/messages` 筛选；点开详情可看钉钉群信息、`request_body.steps` 与 `llm_calls`。

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
