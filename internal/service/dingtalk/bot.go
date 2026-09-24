// Package dingtalk receives robot @mentions over the official Stream protocol
// and replies with RAG-backed AI cards.
package dingtalk

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	channelDingTalk   = "dingtalk"
	streamMinInterval = 300 * time.Millisecond
	corpusMissReply   = "语料库未收录相关知识"
)

type Bot struct {
	cfg         config.DingTalkConfig
	ragTop      int
	maxDistance float64
	previewMax  int
	log         *zap.Logger
	access      *zap.Logger
	db          *gorm.DB
	chat        *chat.Service
	agent       *agent.Service
	rag         *rag.Service
	corpus      *corpus.Service
	api         *openAPI
	dedup       *msgDeduper

	mu           sync.Mutex
	sess         *streamSession
	streamCancel context.CancelFunc
	streamWG     sync.WaitGroup
}

func New(cfg *config.Config, chatSvc *chat.Service, agentSvc *agent.Service, ragSvc *rag.Service, corpusSvc *corpus.Service, log, accessLog *zap.Logger, db *gorm.DB) *Bot {
	if log == nil {
		log = zap.NewNop()
	}
	if accessLog == nil {
		accessLog = log
	}
	if cfg == nil {
		return &Bot{log: log, access: accessLog, db: db, dedup: newMsgDeduper(0)}
	}
	top := cfg.RAG.TopK
	if top <= 0 {
		top = 5
	}
	preview := cfg.Log.BodyPreviewMax
	if preview <= 0 {
		preview = 4096
	}
	return &Bot{
		cfg:         cfg.DingTalk,
		ragTop:      top,
		maxDistance: cfg.RAG.MaxDistance,
		previewMax:  preview,
		log:         log.With(zap.String("component", "dingtalk")),
		access:      accessLog.With(zap.String("component", "dingtalk")),
		db:          db,
		chat:        chatSvc,
		agent:       agentSvc,
		rag:         ragSvc,
		corpus:      corpusSvc,
		api:         newOpenAPI(cfg.DingTalk.ClientID, cfg.DingTalk.ClientSecret),
		dedup:       newMsgDeduper(10 * time.Minute),
	}
}

func (b *Bot) Enabled() bool {
	return b != nil && b.cfg.Enabled
}

func (b *Bot) Start(ctx context.Context) error {
	if !b.Enabled() {
		return nil
	}
	if b.cfg.ClientID == "" || b.cfg.ClientSecret == "" {
		return fmt.Errorf("dingtalk enabled but client_id/client_secret empty")
	}
	if b.chat == nil {
		return fmt.Errorf("dingtalk requires database-backed chat service")
	}
	b.mu.Lock()
	if b.streamCancel != nil {
		b.mu.Unlock()
		b.log.Warn("dingtalk stream already started, ignore duplicate Start",
			zap.Int("pid", os.Getpid()),
		)
		return nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	b.streamCancel = cancel
	b.streamWG.Add(1)
	b.mu.Unlock()

	b.backfillChatsFromConversations()
	go func() {
		defer b.streamWG.Done()
		b.runStream(streamCtx)
	}()
	return nil
}

func (b *Bot) Stop() {
	b.mu.Lock()
	cancel := b.streamCancel
	b.streamCancel = nil
	sess := b.sess
	b.sess = nil
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sess != nil {
		sess.close()
	}
	b.streamWG.Wait()
}

func (b *Bot) runStream(ctx context.Context) {
	b.log.Info("dingtalk stream starting",
		zap.String("client_id_suffix", suffix(b.cfg.ClientID)),
		zap.Int("pid", os.Getpid()),
	)
	b.diagnoseCredentials(ctx)
	backoff := 3 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		sess, err := openStream(ctx, b.cfg.ClientID, b.cfg.ClientSecret)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.log.Error("dingtalk stream handshake failed, will retry",
				zap.Error(err),
				zap.Duration("backoff", backoff),
				zap.String("hint", streamFailHint(err)),
			)
			if !sleepBackoff(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		b.mu.Lock()
		b.sess = sess
		b.mu.Unlock()
		sess.log = b.log
		b.log.Info("dingtalk stream connected")
		err = sess.serve(ctx, func(data *botCallback) {
			b.onMessage(ctx, data)
		})
		b.mu.Lock()
		if b.sess == sess {
			b.sess = nil
		}
		b.mu.Unlock()
		sess.close()
		if ctx.Err() != nil {
			return
		}
		b.log.Error("dingtalk stream stopped, will retry",
			zap.Error(err),
			zap.Duration("backoff", backoff),
			zap.String("hint", streamFailHint(err)),
		)
		if !sleepBackoff(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
	}
}

func sleepBackoff(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func nextBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff > 60*time.Second {
		return 60 * time.Second
	}
	return backoff
}

func (b *Bot) diagnoseCredentials(ctx context.Context) {
	if b.api == nil {
		return
	}
	if _, err := b.api.accessToken(ctx); err != nil {
		b.log.Error("dingtalk client_id/client_secret 换票失败，Stream 也会失败",
			zap.Error(err),
			zap.String("hint", "client_id 必须是应用的 Client ID/AppKey，client_secret 必须是同一应用的 Client Secret/AppSecret，不要填 robot_code"),
		)
		return
	}
	b.log.Info("dingtalk access token ok; if stream still fails, enable Stream on event subscription and robot receive mode")
}

func suffix(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 4 {
		return s
	}
	return "..." + s[len(s)-4:]
}

func streamFailHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "systemError") || strings.Contains(msg, "系统错误") {
		return "凭证已发出握手但被钉钉拒绝：核对 Client ID/Secret 是否成对；开发配置→事件订阅须为 Stream 并验证通道；应用能力→机器人→消息接收模式须为 Stream；应用需发布"
	}
	return "检查到 api.dingtalk.com 与 wss-open-connection.dingtalk.com:443 的出网"
}

func (b *Bot) onMessage(parent context.Context, data *botCallback) {
	if data == nil {
		return
	}
	// 任意入站消息都先归档群/单聊，便于控制台「钉钉群」与请求日志关联。
	b.upsertChat(data)
	if isGroup(data.ConversationType) && !isBotMention(data) {
		b.recordSkip(data, "not_in_at_list")
		return
	}
	if !b.dedup.First(data.MsgID) {
		// 首次投递应已写 request_logs；若落库失败则补记，避免 @ 消息丢失。
		b.log.Info("dingtalk skip: duplicate msg", zap.String("request_id", data.MsgID))
		b.ensureMentionLogged(data)
		return
	}
	tr := b.beginMentionTrace(data)
	go b.handle(parent, data, tr)
}

func (b *Bot) beginMentionTrace(data *botCallback) *msgTrace {
	tr := newMsgTrace(data)
	uid := ""
	if data != nil {
		uid = strings.TrimSpace(data.SenderStaffID)
	}
	tr.uid = uid
	b.recordReceive(tr, data, uid)
	return tr
}

func (b *Bot) ensureMentionLogged(data *botCallback) {
	if b == nil || data == nil || strings.TrimSpace(data.MsgID) == "" {
		return
	}
	if b.hasRequestLog(data.MsgID) {
		return
	}
	tr := b.beginMentionTrace(data)
	tr.status = 204
	tr.outcome = "duplicate"
	tr.errMsg = "duplicate msg"
	b.emitMsgLog(tr)
}

// recordSkip 记录未进入正式处理链路的入站消息（仍须有请求日志与群档案）。
func (b *Bot) recordSkip(data *botCallback, reason string) {
	tr := newMsgTrace(data)
	tr.uid = strings.TrimSpace(data.SenderStaffID)
	tr.status = 204
	tr.outcome = "skipped"
	tr.errMsg = reason
	defer b.emitMsgLog(tr)
	b.recordReceive(tr, data, tr.uid)
	if tr.uid != "" && strings.TrimSpace(data.ConversationID) != "" && b.chat != nil {
		if conv, err := b.chat.GetByChannel(tr.uid, channelDingTalk, data.ConversationID); err == nil && conv != nil {
			tr.conversationID = &conv.ID
		}
	}
}

func (b *Bot) recordReceive(tr *msgTrace, data *botCallback, uid string) {
	detail := receiveDetail(data, uid)
	raw := ""
	senderNick := ""
	msgType := ""
	dingCID := ""
	convType := ""
	inAt := false
	if data != nil {
		raw = previewText(data.Text.Content, b.previewMax)
		senderNick = data.SenderNick
		msgType = data.MsgType
		dingCID = data.ConversationID
		convType = data.ConversationType
		inAt = data.inAtList()
	}
	detail["raw_text"] = raw
	title, _ := detail["conversation_title"].(string)
	b.recordStep(tr, stepReceive, eventReceive, detail,
		zap.String("uid", uid),
		zap.String("sender_nick", senderNick),
		zap.String("msg_type", msgType),
		zap.String("ding_conversation_id", dingCID),
		zap.String("conversation_title", title),
		zap.String("conversation_type", convType),
		zap.Bool("group", isGroup(convType)),
		zap.Bool("is_in_at_list", inAt),
		zap.String("raw_text", raw),
	)
}

func (b *Bot) handle(parent context.Context, data *botCallback, tr *msgTrace) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Minute)
	defer cancel()

	if tr == nil {
		tr = b.beginMentionTrace(data)
	}
	defer b.emitMsgLog(tr)

	uid := tr.uid
	if uid == "" && data != nil {
		uid = strings.TrimSpace(data.SenderStaffID)
		tr.uid = uid
	}
	b.upsertChat(data)
	if uid == "" {
		tr.status = 400
		tr.errMsg = "empty senderStaffId"
		tr.reply = "无法识别发送者 userid（senderStaffId 为空），企业内部群且机器人已发布后才有该字段。"
		_ = b.failReply(ctx, data, tr.reply, tr)
		return
	}
	if !strings.EqualFold(data.MsgType, "text") {
		tr.status = 400
		tr.errMsg = "unsupported msg type"
		tr.reply = "暂只支持文字消息。"
		_ = b.failReply(ctx, data, tr.reply, tr)
		return
	}
	query := cleanQuery(data.Text.Content)
	tr.query = query
	if query == "" {
		tr.status = 400
		tr.errMsg = "empty query"
		tr.reply = "请 @我 并输入问题。"
		_ = b.failReply(ctx, data, tr.reply, tr)
		return
	}

	// ---- 1. 先落会话：uid + 群 conversationId → 最新未删会话，没有则新建 ----
	title := sessionTitle(data)
	conv, err := b.chat.FindOrCreateByChannel(chat.CreateConversationInput{
		UID:              uid,
		Title:            title,
		Channel:          channelDingTalk,
		ChannelSessionID: data.ConversationID,
	})
	if err != nil {
		b.log.Error("find conversation", zap.Error(err), zap.String("request_id", tr.requestID))
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = "创建会话失败，请稍后重试。"
		_ = b.failReply(ctx, data, tr.reply, tr)
		return
	}
	tr.conversationID = &conv.ID
	b.upsertRequestLog(tr)

	// ---- 2. 本轮 RAG；无命中且未要求联网且无历史 → 直接结束（web 模式除外）----
	forceOnline := wantsOnlineSearch(query)
	ragQuery := stripOnlineRequest(query)
	if ragQuery == "" {
		ragQuery = query
	}
	tr.forceOnline = forceOnline
	// 检索词可拼历史；交给 LLM / 落库的仍是本轮用户原句。
	searchQuery := enrichRAGQuery(ragQuery, b.recentHistoryTexts(conv.ID, 6))
	tr.ragQuery = searchQuery
	corpusID, hits, searchIDs := b.retrieve(ctx, searchQuery, data.ConversationID)
	tr.corpusID = corpusID
	tr.hitCount = len(hits)
	tr.hitScores = hitScores(hits)
	tr.sourceNote = b.buildCorpusSourceNote(hits)
	hitsDetail := ragHitLogs(hits, b.previewMax)
	b.recordStep(tr, stepRAG, eventRAG, map[string]any{
		"query":        searchQuery,
		"force_online": forceOnline,
		"hits":         len(hits),
		"scores":       tr.hitScores,
		"corpus_id":    uuidString(corpusID),
		"corpus_ids":   uuidStrings(searchIDs),
		"bound":        len(searchIDs) > 0,
		"hits_detail":  hitsDetail,
	},
		zap.String("query", searchQuery),
		zap.Bool("force_online", forceOnline),
		zap.Int("hits", len(hits)),
		zap.Float64s("scores", tr.hitScores),
		zap.String("corpus_id", uuidString(corpusID)),
		zap.Strings("corpus_ids", uuidStrings(searchIDs)),
		zap.Bool("bound", len(searchIDs) > 0),
		zap.Any("hits_detail", hitsDetail),
	)
	if corpusID != nil && (conv.CorpusID == nil || *conv.CorpusID != *corpusID) {
		if updated, uerr := b.chat.FindOrCreateByChannel(chat.CreateConversationInput{
			UID: uid, Title: title, CorpusID: corpusID,
			Channel: channelDingTalk, ChannelSessionID: data.ConversationID,
		}); uerr == nil && updated != nil {
			conv = updated
		} else {
			conv.CorpusID = corpusID
		}
	}
	if corpusID == nil && conv.CorpusID != nil {
		corpusID = conv.CorpusID
	}
	hasHistory, herr := b.chat.HasMessages(conv.ID)
	if herr != nil {
		b.log.Warn("lookup dingtalk history", zap.Error(herr), zap.String("request_id", tr.requestID))
	}
	// chat/agent：无 RAG、未要求联网、无历史 → 结束。web 交给工具循环自行检索。
	if !b.useWebAgent() && isCorpusMiss(hits, forceOnline) && !hasHistory {
		tr.outcome = "corpus_miss"
		tr.reply = corpusMissReply
		_ = b.failReply(ctx, data, corpusMissReply, tr)
		return
	}
	if hits == nil {
		hits = []rag.Hit{}
	}
	enableSearch := len(hits) == 0 && forceOnline
	tr.enableSearch = enableSearch

	// ---- 3. 带上 RAG 结果 + 会话历史，请求 LLM / 工具 ----
	if b.useWebAgent() {
		b.completeViaWebAgent(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, forceOnline)
		return
	}
	if b.useAgent() {
		b.completeViaAgent(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, hits, enableSearch)
		return
	}
	b.completeViaChat(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, hits, enableSearch)
}

func (b *Bot) useAgent() bool {
	return b != nil && b.cfg.UseAgent() && b.agent != nil
}

func (b *Bot) useWebAgent() bool {
	return b != nil && b.cfg.UseWebAgent() && b.agent != nil
}

func sessionTitle(data *botCallback) string {
	title := chatDisplayTitle(data)
	if data == nil {
		return title
	}
	if nick := strings.TrimSpace(data.SenderNick); nick != "" {
		return title + " · " + nick
	}
	return title
}

func (b *Bot) completeViaChat(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, hits []rag.Hit, enableSearch bool) {
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "llm"
	res, err := b.chat.CompleteStream(ctx, chat.CompleteInput{
		ConversationID: conv.ID,
		UID:            uid,
		Message:        ragQuery,
		RAGEnabled:     len(hits) > 0,
		RAGExplicit:    true,
		CorpusID:       corpusID,
		CorpusIDs:      corpusIDs,
		RAGHits:        hits,
		TopK:           b.ragTop,
		EnableSearch:   enableSearch,
		RequestID:      tr.requestID,
		LogLLMRequest: func(provider, model string, messages []llm.Message) {
			turns := llmTurnLogs(messages, b.previewMax)
			b.recordStep(tr, stepLLMRequest, eventLLMRequest, map[string]any{
				"provider":        provider,
				"model":           model,
				"enable_search":   enableSearch,
				"rag_enabled":     len(hits) > 0,
				"conversation_id": conv.ID.String(),
				"messages":        len(messages),
				"conversation":    turns,
			},
				zap.String("provider", provider),
				zap.String("model", model),
				zap.Bool("enable_search", enableSearch),
				zap.Bool("rag_enabled", len(hits) > 0),
				zap.String("conversation_id", conv.ID.String()),
				zap.Int("messages", len(messages)),
				zap.Any("conversation", turns),
			)
		},
	}, func(delta string) error {
		acc.WriteString(delta)
		now := time.Now()
		if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
			return nil
		}
		lastFlush = now
		return streamer.update(acc.String(), false)
	})
	if err != nil {
		b.log.Error("complete stream", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := res.Content
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

func (b *Bot) completeViaAgent(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, hits []rag.Hit, enableSearch bool) {
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "agent"
	cid := conv.ID
	toolTrace := make([]map[string]any, 0, 8)
	res, err := b.agent.Run(ctx, agent.RunInput{
		ConversationID: &cid,
		UID:            uid,
		Input:          ragQuery,
		CorpusID:       corpusID,
		CorpusIDs:      corpusIDs,
		TopK:           b.ragTop,
		RAGHits:        hits,
		EnableSearch:   enableSearch,
		RequestID:      tr.requestID,
		Stream:         true,
	}, func(ev agent.Event) error {
		switch ev.Type {
		case "delta":
			content, _ := ev.Payload["content"].(string)
			acc.WriteString(content)
			now := time.Now()
			if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
				return nil
			}
			lastFlush = now
			return streamer.update(acc.String(), false)
		case "tool_call":
			name, _ := ev.Payload["name"].(string)
			args, _ := ev.Payload["arguments"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":      "tool_call",
				"name":      name,
				"arguments": previewText(args, b.previewMax),
			})
			hint := "正在调用工具…"
			if name == "dbconn" {
				hint = "正在查询业务库…"
			} else if name == "knowledge_search" {
				hint = "正在检索知识库…"
			}
			_ = streamer.update(hint, false)
		case "tool_result":
			name, _ := ev.Payload["name"].(string)
			content, _ := ev.Payload["content"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":    "tool_result",
				"name":    name,
				"content": previewText(content, b.previewMax),
			})
		}
		return nil
	})
	detail := map[string]any{
		"enable_search":   enableSearch,
		"rag_enabled":     len(hits) > 0,
		"conversation_id": conv.ID.String(),
		"tools":           toolTrace,
	}
	if res != nil {
		rid := res.RunID
		tr.agentRunID = &rid
		detail["run_id"] = res.RunID.String()
		detail["step_count"] = res.StepCount
		detail["status"] = res.Status
		if dbSteps := b.agentStepsForLog(res.RunID); len(dbSteps) > 0 {
			detail["steps"] = dbSteps
		}
	}
	b.recordStep(tr, stepLLMRequest, eventAgent, detail,
		zap.String("run_id", anyString(detail["run_id"])),
		zap.Int("tool_events", len(toolTrace)),
		zap.String("conversation_id", conv.ID.String()),
	)
	if err != nil {
		b.log.Error("agent run", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := ""
	if res != nil {
		text = res.Output
	}
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

// completeViaWebAgent 薄包装：与控制台 Agent 共用 agent.Run，并保留钉钉会话上下文。
// - 带 ConversationID：多轮追问（如补标签）可用历史
// - RAGHits 传空切片：跳过 system 预注入（避免摘录+旧结论让模型跳过工具）；检索范围仍经 CorpusIDs 交给工具
// - 现有 completeViaAgent（钉钉预检索注入）保持不变
func (b *Bot) completeViaWebAgent(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, input string, corpusID *uuid.UUID, corpusIDs []uuid.UUID, forceOnline bool) {
	streamer := b.newStreamer(ctx, data, tr)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "web_agent"
	cid := conv.ID
	enableSearch := forceOnline
	tr.enableSearch = enableSearch
	toolTrace := make([]map[string]any, 0, 8)
	topK := b.ragTop
	if topK <= 0 {
		topK = 5
	}
	res, err := b.agent.Run(ctx, agent.RunInput{
		ConversationID: &cid,
		UID:            uid,
		Input:          input,
		CorpusID:       corpusID,
		CorpusIDs:      corpusIDs,
		TopK:           topK,
		// 非 nil 空切片：禁止 agent.retrieveHits 写入 system；工具侧仍用 CorpusIDs。
		RAGHits:      []rag.Hit{},
		EnableSearch: enableSearch,
		RequestID:    tr.requestID,
		Stream:       true,
	}, func(ev agent.Event) error {
		switch ev.Type {
		case "delta":
			content, _ := ev.Payload["content"].(string)
			acc.WriteString(content)
			now := time.Now()
			if now.Sub(lastFlush) < streamMinInterval && acc.Len() < 80 {
				return nil
			}
			lastFlush = now
			return streamer.update(acc.String(), false)
		case "tool_call":
			name, _ := ev.Payload["name"].(string)
			args, _ := ev.Payload["arguments"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":      "tool_call",
				"name":      name,
				"arguments": previewText(args, b.previewMax),
			})
			hint := "正在调用工具…"
			if name == "dbconn" {
				hint = "正在查询业务库…"
			} else if name == "knowledge_search" {
				hint = "正在检索知识库…"
			}
			_ = streamer.update(hint, false)
		case "tool_result":
			name, _ := ev.Payload["name"].(string)
			content, _ := ev.Payload["content"].(string)
			toolTrace = append(toolTrace, map[string]any{
				"kind":    "tool_result",
				"name":    name,
				"content": previewText(content, b.previewMax),
			})
		}
		return nil
	})
	detail := map[string]any{
		"mode":            "web",
		"enable_search":   enableSearch,
		"conversation_id": conv.ID.String(),
		"corpus_id":       uuidString(corpusID),
		"corpus_ids":      uuidStrings(corpusIDs),
		"rag_preinject":   false,
		"tools":           toolTrace,
	}
	if res != nil {
		rid := res.RunID
		tr.agentRunID = &rid
		detail["run_id"] = res.RunID.String()
		detail["step_count"] = res.StepCount
		detail["status"] = res.Status
		if dbSteps := b.agentStepsForLog(res.RunID); len(dbSteps) > 0 {
			detail["steps"] = dbSteps
		}
	}
	b.recordStep(tr, stepLLMRequest, eventWebAgent, detail,
		zap.String("run_id", anyString(detail["run_id"])),
		zap.Int("tool_events", len(toolTrace)),
		zap.String("conversation_id", conv.ID.String()),
	)
	if err != nil {
		b.log.Error("web agent run", zap.Error(err), zap.String("request_id", tr.requestID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = final
		_ = streamer.finish(final, true)
		return
	}
	text := ""
	if res != nil {
		text = res.Output
	}
	if strings.TrimSpace(text) == "" {
		text = acc.String()
	}
	if strings.TrimSpace(text) == "" {
		text = "没有生成内容。"
	}
	tr.reply = text
	_ = streamer.finish(text, false)
}

func (b *Bot) agentStepsForLog(runID uuid.UUID) []map[string]any {
	if b == nil || b.agent == nil || runID == uuid.Nil {
		return nil
	}
	_, steps, err := b.agent.GetRun(runID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(steps))
	for _, st := range steps {
		item := map[string]any{
			"step_index": st.StepIndex,
			"kind":       st.Kind,
		}
		if st.ToolName != "" {
			item["tool_name"] = st.ToolName
		}
		if st.InputJSON != "" && st.InputJSON != "{}" {
			item["input"] = previewText(st.InputJSON, b.previewMax)
		}
		if st.OutputText != "" {
			item["output"] = previewText(st.OutputText, b.previewMax)
		}
		out = append(out, item)
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (b *Bot) retrieve(ctx context.Context, query, conversationID string) (*uuid.UUID, []rag.Hit, []uuid.UUID) {
	if b.rag == nil || b.corpus == nil {
		return nil, nil, nil
	}
	bound := b.boundCorpora(conversationID)
	pool := bound
	if len(pool) == 0 {
		all, err := b.corpus.List()
		if err != nil {
			b.log.Warn("list corpora", zap.Error(err))
			return nil, nil, nil
		}
		if len(all) == 0 {
			return nil, nil, nil
		}
		pool = all
	}
	searchIn := corporaForRetrieve(pool, query)
	var pinned *uuid.UUID
	if matched := matchCorpora(query, searchIn); len(matched) > 0 {
		id := matched[0].ID
		pinned = &id
	}
	var scope []uuid.UUID
	if len(bound) > 0 {
		scope = corpusIDs(bound)
	}
	hits, err := b.rag.SearchInCorpora(ctx, corpusIDs(searchIn), query, b.ragTop)
	if err != nil {
		b.log.Warn("rag search", zap.Error(err))
		return pinned, []rag.Hit{}, scope
	}
	hits = filterRelevant(hits, b.maxDistance)
	if hits == nil {
		hits = []rag.Hit{}
	}
	if best := bestCorpusID(hits); best != nil {
		pinned = best
	}
	return pinned, hits, scope
}

// recentHistoryTexts 取近期 user/assistant 正文，供短追问扩写检索词。
func (b *Bot) recentHistoryTexts(conversationID uuid.UUID, limit int) []string {
	if b == nil || b.chat == nil || conversationID == uuid.Nil {
		return nil
	}
	msgs, err := b.chat.RecentMessages(conversationID, limit)
	if err != nil || len(msgs) == 0 {
		return nil
	}
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		c := strings.TrimSpace(m.Content)
		if c == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

// enrichRAGQuery 短句追问时把近期对话拼进检索词，避免只搜到标签/合同等碎片。
func enrichRAGQuery(query string, history []string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return q
	}
	if utf8.RuneCountInString(q) > 48 {
		return q
	}
	var parts []string
	for _, h := range history {
		c := strings.TrimSpace(h)
		if c == "" || c == q {
			continue
		}
		r := []rune(c)
		if len(r) > 80 {
			c = string(r[:80])
		}
		parts = append(parts, c)
	}
	if len(parts) == 0 {
		return q
	}
	if len(parts) > 4 {
		parts = parts[len(parts)-4:]
	}
	return q + "\n" + strings.Join(parts, "\n")
}

type streamer struct {
	bot      *Bot
	data     *botCallback
	tr       *msgTrace
	outTrack string
	cardOK   bool
}

func (b *Bot) newStreamer(ctx context.Context, data *botCallback, tr *msgTrace) *streamer {
	s := &streamer{bot: b, data: data, tr: tr}
	if strings.TrimSpace(b.cfg.CardTemplateID) == "" || b.api == nil {
		return s
	}
	id, err := b.api.createCard(ctx, b.cfg.CardTemplateID, data.ConversationID, data.ConversationType, data.SenderStaffID, "正在思考…")
	if err != nil {
		b.log.Warn("create card, will fallback", zap.Error(err))
		return s
	}
	s.outTrack = id
	s.cardOK = true
	return s
}

func (s *streamer) update(content string, finalize bool) error {
	if !s.cardOK {
		return nil
	}
	// 流式中间帧不加语料脚注，避免末尾摘要反复跳动；finalize 由 finish 统一装饰。
	content = s.bot.withReplyTag(s.bot.sanitizeOutbound(content))
	err := s.bot.api.streamCard(context.Background(), s.outTrack, content, finalize, false)
	if err != nil {
		s.bot.log.Warn("stream card", zap.Error(err))
		s.cardOK = false
	}
	return nil
}

func (s *streamer) finish(content string, isError bool) error {
	ctx := context.Background()
	content = s.bot.decorateOutbound(s.tr, content)
	// 先落库再出站：避免群里已回复、控制台/Agent 历史却无记录。
	s.bot.persistOutbound(s.tr, content)
	if s.cardOK {
		if err := s.bot.api.streamCard(ctx, s.outTrack, content, true, isError); err == nil {
			return nil
		}
		s.cardOK = false
	}
	return s.bot.sendOutbound(ctx, s.data, content)
}

// persistOutbound 出站前强制写入 request_log，并补一条 assistant 消息（若会话里还没有同文）。
func (b *Bot) persistOutbound(tr *msgTrace, text string) {
	if b == nil {
		return
	}
	text = strings.TrimSpace(text)
	if tr != nil {
		tr.reply = text
		if tr.outcome == "" {
			tr.outcome = "agent"
		}
		b.upsertRequestLog(tr)
		rid := tr.requestID
		b.stepLog(rid, stepResult, "dingtalk.outbound",
			zap.String("request_id", rid),
			zap.String("reply", previewText(text, b.previewMax)),
			zap.String("conversation_id", uuidString(tr.conversationID)),
			zap.Int("pid", os.Getpid()),
		)
		if tr.conversationID != nil && text != "" && b.db != nil {
			var n int64
			_ = b.db.Model(&model.Message{}).
				Where("conversation_id = ? AND role = ? AND content = ?", *tr.conversationID, "assistant", text).
				Count(&n).Error
			if n == 0 {
				msg := model.Message{ConversationID: *tr.conversationID, Role: "assistant", Content: text}
				if err := b.db.Create(&msg).Error; err != nil {
					b.log.Error("persist outbound assistant message", zap.Error(err), zap.String("request_id", rid))
				}
			}
		}
		return
	}
	if b.log != nil {
		b.log.Error("dingtalk outbound without msgTrace — reply would be invisible in console",
			zap.String("reply", previewText(text, 120)),
			zap.Int("pid", os.Getpid()),
		)
	}
}

func (b *Bot) sanitizeOutbound(text string) string {
	if !agent.IsToolEvasionReply(text) {
		return text
	}
	if b != nil && b.log != nil {
		b.log.Warn("blocked tool-evasion outbound reply",
			zap.String("preview", previewText(text, 120)),
			zap.Int("pid", os.Getpid()),
		)
	}
	return agent.ToolEvasionFallback
}

func (b *Bot) failReply(ctx context.Context, data *botCallback, text string, tr *msgTrace) error {
	text = b.decorateOutbound(tr, text)
	b.persistOutbound(tr, text)
	return b.sendOutbound(ctx, data, text)
}

// decorateOutbound 出站统一装饰：拦截推诿 → 语料来源脚注 → 实例 reply_tag。
func (b *Bot) decorateOutbound(tr *msgTrace, text string) string {
	text = b.sanitizeOutbound(text)
	note := ""
	if tr != nil {
		note = tr.sourceNote
	}
	text = appendSourceNote(text, note)
	return b.withReplyTag(text)
}

// buildCorpusSourceNote 根据本轮 RAG 命中生成简短来源说明。
func (b *Bot) buildCorpusSourceNote(hits []rag.Hit) string {
	if len(hits) == 0 {
		return ""
	}
	names := b.corpusNamesForHits(hits)
	hitN := len(hits)
	switch {
	case len(names) == 0:
		return fmt.Sprintf("来源：语料库（命中 %d 条）", hitN)
	case len(names) == 1:
		return fmt.Sprintf("来源：%s（命中 %d 条）", names[0], hitN)
	default:
		shown := names
		extra := 0
		if len(shown) > 3 {
			extra = len(shown) - 3
			shown = shown[:3]
		}
		s := "来源：" + strings.Join(shown, "、")
		if extra > 0 {
			s += " 等" + strconv.Itoa(extra) + "个"
		}
		return fmt.Sprintf("%s（命中 %d 条）", s, hitN)
	}
}

func (b *Bot) corpusNamesForHits(hits []rag.Hit) []string {
	if b == nil || b.db == nil || len(hits) == 0 {
		return nil
	}
	order := make([]uuid.UUID, 0, 4)
	seen := map[uuid.UUID]struct{}{}
	for _, h := range hits {
		if h.CorpusID == uuid.Nil {
			continue
		}
		if _, ok := seen[h.CorpusID]; ok {
			continue
		}
		seen[h.CorpusID] = struct{}{}
		order = append(order, h.CorpusID)
	}
	if len(order) == 0 {
		return nil
	}
	var rows []model.Corpus
	if err := b.db.Select("id", "name").Where("id IN ?", order).Find(&rows).Error; err != nil {
		return nil
	}
	byID := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		byID[r.ID] = strings.TrimSpace(r.Name)
	}
	out := make([]string, 0, len(order))
	for _, id := range order {
		if n := byID[id]; n != "" {
			out = append(out, n)
		}
	}
	return out
}

func appendSourceNote(text, note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return text
	}
	if strings.Contains(text, note) {
		return text
	}
	return strings.TrimRight(text, "\n") + "\n\n" + note
}

func (b *Bot) sendOutbound(ctx context.Context, data *botCallback, text string) error {
	if data == nil {
		return fmt.Errorf("nil callback")
	}
	// tag 已在 finish/failReply 加上；此处再幂等一次防漏。
	text = b.withReplyTag(text)
	title := "AI 回复"
	var atIDs []string
	if uid := strings.TrimSpace(data.SenderStaffID); uid != "" && isGroup(data.ConversationType) {
		atIDs = []string{uid}
		// 仅出站加 @，不进 persistOutbound，避免历史里刷 staffId。
		text = appendMarkdownAt(text, uid)
	}
	if err := replyWebhook(ctx, data.SessionWebhook, title, text, atIDs); err == nil {
		return nil
	} else if b.log != nil {
		b.log.Warn("session webhook reply", zap.Error(err))
	}
	if isGroup(data.ConversationType) && b.api != nil && b.cfg.ClientID != "" {
		if err := b.api.sendGroupMarkdown(ctx, data.ConversationID, title, text); err != nil {
			b.log.Error("group markdown fallback", zap.Error(err))
			return err
		}
		return nil
	}
	return fmt.Errorf("no reply channel")
}

// appendMarkdownAt 按钉钉约定在正文末尾补 @userid，配合 at.atUserIds 才会显示蓝字 @。
func appendMarkdownAt(text, staffID string) string {
	staffID = strings.TrimSpace(staffID)
	if staffID == "" {
		return text
	}
	token := "@" + staffID
	if strings.Contains(text, token) {
		return text
	}
	return strings.TrimRight(text, "\n") + "\n\n" + token
}

func (b *Bot) withReplyTag(text string) string {
	if b == nil {
		return text
	}
	tag := strings.TrimSpace(b.cfg.ReplyTag)
	if tag == "" {
		return text
	}
	marker := "〔" + tag + "〕"
	if strings.Contains(text, marker) {
		return text
	}
	text = strings.TrimRight(text, "\n")
	return text + "\n\n" + marker
}
