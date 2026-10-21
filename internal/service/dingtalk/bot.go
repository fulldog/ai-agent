// Package dingtalk receives robot @mentions over the official Stream protocol
// and replies with RAG-backed AI cards.
package dingtalk

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	channelDingTalk = "dingtalk"
	corpusMissReply = "语料库未收录相关知识"
)

type Bot struct {
	cfg         config.DingTalkConfig
	ragTop      int
	maxDistance float64
	previewMax  int
	log         *zap.Logger
	access      *zap.Logger
	db          *gorm.DB
	llm         config.LLMConfig
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

	modeMu             sync.RWMutex
	replyModeFromStore bool
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
	b := &Bot{
		cfg:         cfg.DingTalk,
		ragTop:      top,
		maxDistance: cfg.RAG.MaxDistance,
		previewMax:  preview,
		log:         log.With(zap.String("component", "dingtalk")),
		access:      accessLog.With(zap.String("component", "dingtalk")),
		db:          db,
		llm:         cfg.LLM,
		chat:        chatSvc,
		agent:       agentSvc,
		rag:         ragSvc,
		corpus:      corpusSvc,
		api:         newOpenAPI(cfg.DingTalk.ClientID, cfg.DingTalk.ClientSecret),
		dedup:       newMsgDeduper(10 * time.Minute),
	}
	b.loadStoredReplyMode()
	return b
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
	// 未绑定语料库的单聊不检索，直接请求大模型并打开联网。
	forceOnline := wantsOnlineSearch(query)
	ragQuery := stripOnlineRequest(query)
	if ragQuery == "" {
		ragQuery = query
	}
	tr.forceOnline = forceOnline
	tr.ragQuery = ragQuery
	skipRAG := unboundDirectChat(data.ConversationType, len(b.boundCorpora(data.ConversationID)))
	var corpusID *uuid.UUID
	var hits []rag.Hit
	var searchIDs []uuid.UUID
	if skipRAG {
		hits = []rag.Hit{}
	} else {
		// RAG 只检索本轮用户输入；历史消息仅在下一阶段作为 LLM 上下文。
		corpusID, hits, searchIDs = b.retrieve(ctx, ragQuery, data.ConversationID)
	}
	tr.corpusID = corpusID
	tr.hitCount = len(hits)
	tr.hitScores = hitScores(hits)
	tr.sourceNote = b.buildCorpusSourceNote(hits)
	hitsDetail := ragHitLogs(hits, b.previewMax)
	b.recordStep(tr, stepRAG, eventRAG, map[string]any{
		"query":        ragQuery,
		"force_online": forceOnline,
		"skip_rag":     skipRAG,
		"hits":         len(hits),
		"scores":       tr.hitScores,
		"corpus_id":    uuidString(corpusID),
		"corpus_ids":   uuidStrings(searchIDs),
		"bound":        len(searchIDs) > 0,
		"hits_detail":  hitsDetail,
	},
		zap.String("query", ragQuery),
		zap.Bool("force_online", forceOnline),
		zap.Bool("skip_rag", skipRAG),
		zap.Int("hits", len(hits)),
		zap.Float64s("scores", tr.hitScores),
		zap.String("corpus_id", uuidString(corpusID)),
		zap.Strings("corpus_ids", uuidStrings(searchIDs)),
		zap.Bool("bound", len(searchIDs) > 0),
		zap.Any("hits_detail", hitsDetail),
	)
	if !skipRAG && corpusID != nil && (conv.CorpusID == nil || *conv.CorpusID != *corpusID) {
		if updated, uerr := b.chat.FindOrCreateByChannel(chat.CreateConversationInput{
			UID: uid, Title: title, CorpusID: corpusID,
			Channel: channelDingTalk, ChannelSessionID: data.ConversationID,
		}); uerr == nil && updated != nil {
			conv = updated
		} else {
			conv.CorpusID = corpusID
		}
	}
	if !skipRAG && corpusID == nil && conv.CorpusID != nil {
		corpusID = conv.CorpusID
	}
	hasHistory, herr := b.chat.HasMessages(conv.ID)
	if herr != nil {
		b.log.Warn("lookup dingtalk history", zap.Error(herr), zap.String("request_id", tr.requestID))
	}
	// chat/agent：无 RAG、未要求联网、无历史、且未挂 dbconn → 结束。
	// 挂了 dbconn 时继续，让模型决定是否查库。web 交给工具循环自行检索。未绑定语料库的单聊不走这条早退。
	convID := ""
	if data != nil {
		convID = data.ConversationID
	}
	hasDB := slices.Contains(b.chatTools(convID), "dbconn")
	if !skipRAG && !b.useWebAgentFor(convID) && stopForCorpusMiss(hits, forceOnline, hasHistory, hasDB) {
		tr.outcome = "corpus_miss"
		tr.reply = corpusMissReply
		_ = b.failReply(ctx, data, corpusMissReply, tr)
		return
	}
	if hits == nil {
		hits = []rag.Hit{}
	}
	enableSearch := skipRAG || (len(hits) == 0 && forceOnline)
	tr.enableSearch = enableSearch

	// ---- 3. 带上 RAG 结果 + 会话历史，请求 LLM / 工具 ----
	if b.useWebAgentFor(convID) {
		b.completeViaWebAgent(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, enableSearch, skipRAG)
		return
	}
	if b.useAgentFor(convID) {
		b.completeViaAgent(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, hits, enableSearch, skipRAG)
		return
	}
	b.completeViaChat(ctx, data, tr, conv, uid, ragQuery, corpusID, searchIDs, hits, enableSearch, skipRAG)
}

func (b *Bot) useAgent() bool {
	return b.useAgentFor("")
}

func (b *Bot) useWebAgent() bool {
	return b.useWebAgentFor("")
}

func (b *Bot) useAgentFor(conversationID string) bool {
	return b != nil && b.chatReplyMode(conversationID) == config.DingTalkReplyAgent && b.agent != nil
}

func (b *Bot) useWebAgentFor(conversationID string) bool {
	return b != nil && b.chatReplyMode(conversationID) == config.DingTalkReplyWeb && b.agent != nil
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
