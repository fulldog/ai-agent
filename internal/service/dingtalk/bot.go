// Package dingtalk receives robot @mentions over the official Stream protocol
// and replies with RAG-backed AI cards.
package dingtalk

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
	persistBody bool
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

	mu   sync.Mutex
	sess *streamSession
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
		persistBody: cfg.RequestLog.PersistBody,
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
	go b.runStream(ctx)
	return nil
}

func (b *Bot) runStream(ctx context.Context) {
	b.log.Info("dingtalk stream starting", zap.String("client_id_suffix", suffix(b.cfg.ClientID)))
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

func (b *Bot) Stop() {
	b.mu.Lock()
	sess := b.sess
	b.sess = nil
	b.mu.Unlock()
	if sess != nil {
		sess.close()
	}
}

func (b *Bot) onMessage(parent context.Context, data *botCallback) {
	if data == nil {
		return
	}
	if isGroup(data.ConversationType) && !data.IsInAtList {
		b.log.Debug("dingtalk skip: not in at list",
			zap.String("request_id", data.MsgID),
			zap.String("conversation_id", data.ConversationID),
		)
		return
	}
	if !b.dedup.First(data.MsgID) {
		b.log.Info("dingtalk skip: duplicate msg", zap.String("request_id", data.MsgID))
		return
	}
	go b.handle(parent, data)
}

func (b *Bot) handle(parent context.Context, data *botCallback) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Minute)
	defer cancel()

	tr := newMsgTrace(data)
	defer b.emitMsgLog(tr)

	uid := strings.TrimSpace(data.SenderStaffID)
	tr.uid = uid
	b.recordStep(tr, stepReceive, eventReceive, map[string]any{
		"uid":                  uid,
		"sender_nick":          data.SenderNick,
		"msg_type":             data.MsgType,
		"ding_conversation_id": data.ConversationID,
		"conversation_type":    data.ConversationType,
		"group":                isGroup(data.ConversationType),
		"raw_text":             previewText(data.Text.Content, b.previewMax),
	},
		zap.String("uid", uid),
		zap.String("sender_nick", data.SenderNick),
		zap.String("msg_type", data.MsgType),
		zap.String("ding_conversation_id", data.ConversationID),
		zap.String("conversation_type", data.ConversationType),
		zap.Bool("group", isGroup(data.ConversationType)),
		zap.String("raw_text", previewText(data.Text.Content, b.previewMax)),
	)
	if uid == "" {
		tr.status = 400
		tr.errMsg = "empty senderStaffId"
		tr.reply = "无法识别发送者 userid（senderStaffId 为空），企业内部群且机器人已发布后才有该字段。"
		_ = b.failReply(ctx, data, tr.reply)
		return
	}
	if !strings.EqualFold(data.MsgType, "text") {
		tr.status = 400
		tr.errMsg = "unsupported msg type"
		tr.reply = "暂只支持文字消息。"
		_ = b.failReply(ctx, data, tr.reply)
		return
	}
	query := cleanQuery(data.Text.Content)
	tr.query = query
	if query == "" {
		tr.status = 400
		tr.errMsg = "empty query"
		tr.reply = "请 @我 并输入问题。"
		_ = b.failReply(ctx, data, tr.reply)
		return
	}

	forceOnline := wantsOnlineSearch(query)
	ragQuery := stripOnlineRequest(query)
	if ragQuery == "" {
		ragQuery = query
	}
	tr.ragQuery = ragQuery
	tr.forceOnline = forceOnline
	corpusID, hits := b.retrieve(ctx, ragQuery)
	tr.corpusID = corpusID
	tr.hitCount = len(hits)
	tr.hitScores = hitScores(hits)
	hitsDetail := ragHitLogs(hits, b.previewMax)
	b.recordStep(tr, stepRAG, eventRAG, map[string]any{
		"query":        ragQuery,
		"force_online": forceOnline,
		"hits":         len(hits),
		"scores":       tr.hitScores,
		"corpus_id":    uuidString(corpusID),
		"hits_detail":  hitsDetail,
	},
		zap.String("query", ragQuery),
		zap.Bool("force_online", forceOnline),
		zap.Int("hits", len(hits)),
		zap.Float64s("scores", tr.hitScores),
		zap.String("corpus_id", uuidString(corpusID)),
		zap.Any("hits_detail", hitsDetail),
	)
	if isCorpusMiss(hits, forceOnline) {
		tr.outcome = "corpus_miss"
		tr.reply = corpusMissReply
		_ = b.failReply(ctx, data, corpusMissReply)
		return
	}
	title := strings.TrimSpace(data.ConversationTitle)
	if title == "" {
		if isGroup(data.ConversationType) {
			title = "钉钉群聊"
		} else {
			title = "钉钉单聊"
		}
	}
	if data.SenderNick != "" {
		title = title + " · " + data.SenderNick
	}
	conv, err := b.chat.FindOrCreateByChannel(chat.CreateConversationInput{
		UID:              uid,
		Title:            title,
		CorpusID:         corpusID,
		Channel:          channelDingTalk,
		ChannelSessionID: data.ConversationID,
	})
	if err != nil {
		b.log.Error("find conversation", zap.Error(err), zap.String("request_id", tr.requestID))
		tr.status = 500
		tr.errMsg = err.Error()
		tr.reply = "创建会话失败，请稍后重试。"
		_ = b.failReply(ctx, data, tr.reply)
		return
	}
	tr.conversationID = &conv.ID
	enableSearch := len(hits) == 0 && forceOnline
	tr.enableSearch = enableSearch

	if b.useAgent() {
		b.completeViaAgent(ctx, data, tr, conv, uid, ragQuery, corpusID, hits, enableSearch)
		return
	}
	b.completeViaChat(ctx, data, tr, conv, uid, ragQuery, corpusID, hits, enableSearch)
}

func (b *Bot) useAgent() bool {
	return b != nil && b.cfg.UseAgent() && b.agent != nil
}

func (b *Bot) completeViaChat(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, hits []rag.Hit, enableSearch bool) {
	streamer := b.newStreamer(ctx, data)
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

func (b *Bot) completeViaAgent(ctx context.Context, data *botCallback, tr *msgTrace, conv *model.Conversation, uid, ragQuery string, corpusID *uuid.UUID, hits []rag.Hit, enableSearch bool) {
	streamer := b.newStreamer(ctx, data)
	var acc strings.Builder
	lastFlush := time.Now()
	tr.outcome = "agent"
	cid := conv.ID
	res, err := b.agent.Run(ctx, agent.RunInput{
		ConversationID: &cid,
		UID:            uid,
		Input:          ragQuery,
		CorpusID:       corpusID,
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
			hint := "正在调用工具…"
			if name == "dbconn" {
				hint = "正在查询业务库…"
			} else if name == "knowledge_search" {
				hint = "正在检索知识库…"
			}
			_ = streamer.update(hint, false)
		}
		return nil
	})
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
		b.recordStep(tr, stepLLMRequest, eventAgent, map[string]any{
			"run_id":          res.RunID.String(),
			"step_count":      res.StepCount,
			"status":          res.Status,
			"enable_search":   enableSearch,
			"rag_enabled":     len(hits) > 0,
			"conversation_id": conv.ID.String(),
		},
			zap.String("run_id", res.RunID.String()),
			zap.Int("step_count", res.StepCount),
			zap.String("conversation_id", conv.ID.String()),
		)
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

func (b *Bot) retrieve(ctx context.Context, query string) (*uuid.UUID, []rag.Hit) {
	if b.rag == nil || b.corpus == nil {
		return nil, nil
	}
	all, err := b.corpus.List()
	if err != nil {
		b.log.Warn("list corpora", zap.Error(err))
		return nil, nil
	}
	if len(all) == 0 {
		return nil, nil
	}
	matched := matchCorpora(query, all)
	searchIn := all
	var pinned *uuid.UUID
	if len(matched) > 0 {
		searchIn = matched
		id := matched[0].ID
		pinned = &id
	}
	hits, err := b.rag.SearchInCorpora(ctx, corpusIDs(searchIn), query, b.ragTop)
	if err != nil {
		b.log.Warn("rag search", zap.Error(err))
		return pinned, nil
	}
	hits = filterRelevant(hits, b.maxDistance)
	if best := bestCorpusID(hits); best != nil {
		pinned = best
	}
	return pinned, hits
}

type streamer struct {
	bot      *Bot
	data     *botCallback
	outTrack string
	cardOK   bool
}

func (b *Bot) newStreamer(ctx context.Context, data *botCallback) *streamer {
	s := &streamer{bot: b, data: data}
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
	err := s.bot.api.streamCard(context.Background(), s.outTrack, content, finalize, false)
	if err != nil {
		s.bot.log.Warn("stream card", zap.Error(err))
		s.cardOK = false
	}
	return nil
}

func (s *streamer) finish(content string, isError bool) error {
	ctx := context.Background()
	if s.cardOK {
		if err := s.bot.api.streamCard(ctx, s.outTrack, content, true, isError); err == nil {
			return nil
		}
		s.cardOK = false
	}
	return s.bot.failReply(ctx, s.data, content)
}

func (b *Bot) failReply(ctx context.Context, data *botCallback, text string) error {
	title := "AI 回复"
	if err := replyWebhook(ctx, data.SessionWebhook, title, text); err == nil {
		return nil
	} else {
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
