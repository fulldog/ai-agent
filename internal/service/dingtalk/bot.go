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
	"github.com/webapp/go-app/ai-agent/internal/service/chat"
	"github.com/webapp/go-app/ai-agent/internal/service/corpus"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
)

const (
	channelDingTalk   = "dingtalk"
	streamMinInterval = 300 * time.Millisecond
)

type Bot struct {
	cfg    config.DingTalkConfig
	ragTop int
	log    *zap.Logger
	chat   *chat.Service
	rag    *rag.Service
	corpus *corpus.Service
	api    *openAPI
	dedup  *msgDeduper

	mu   sync.Mutex
	sess *streamSession
}

func New(cfg *config.Config, chatSvc *chat.Service, ragSvc *rag.Service, corpusSvc *corpus.Service, log *zap.Logger) *Bot {
	if log == nil {
		log = zap.NewNop()
	}
	if cfg == nil {
		return &Bot{log: log, dedup: newMsgDeduper(0)}
	}
	top := cfg.RAG.TopK
	if top <= 0 {
		top = 5
	}
	return &Bot{
		cfg:    cfg.DingTalk,
		ragTop: top,
		log:    log.With(zap.String("component", "dingtalk")),
		chat:   chatSvc,
		rag:    ragSvc,
		corpus: corpusSvc,
		api:    newOpenAPI(cfg.DingTalk.ClientID, cfg.DingTalk.ClientSecret),
		dedup:  newMsgDeduper(10 * time.Minute),
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
		return
	}
	if !b.dedup.First(data.MsgID) {
		return
	}
	go b.handle(parent, data)
}

func (b *Bot) handle(parent context.Context, data *botCallback) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Minute)
	defer cancel()

	uid := strings.TrimSpace(data.SenderStaffID)
	if uid == "" {
		_ = b.failReply(ctx, data, "无法识别发送者 userid（senderStaffId 为空），企业内部群且机器人已发布后才有该字段。")
		return
	}
	if !strings.EqualFold(data.MsgType, "text") {
		_ = b.failReply(ctx, data, "暂只支持文字消息。")
		return
	}
	query := cleanQuery(data.Text.Content)
	if query == "" {
		_ = b.failReply(ctx, data, "请 @我 并输入问题。")
		return
	}

	corpusID, hits := b.retrieve(ctx, query)
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
		b.log.Error("find conversation", zap.Error(err))
		_ = b.failReply(ctx, data, "创建会话失败，请稍后重试。")
		return
	}

	streamer := b.newStreamer(ctx, data)
	var acc strings.Builder
	lastFlush := time.Now()
	res, err := b.chat.CompleteStream(ctx, chat.CompleteInput{
		ConversationID: conv.ID,
		UID:            uid,
		Message:        query,
		RAGEnabled:     true,
		CorpusID:       corpusID,
		RAGHits:        hits,
		TopK:           b.ragTop,
		RequestID:      data.MsgID,
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
		b.log.Error("complete stream", zap.Error(err), zap.String("msg_id", data.MsgID))
		final := acc.String()
		if final == "" {
			final = llm.PublicMessage(err)
		}
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
	_ = streamer.finish(text, false)
}

func (b *Bot) retrieve(ctx context.Context, query string) (*uuid.UUID, []rag.Hit) {
	if b.rag == nil || b.corpus == nil {
		return nil, nil
	}
	all, err := b.corpus.List()
	if err != nil || len(all) == 0 {
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
