package dingtalk

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
)

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

// persistOutbound 出站前写入 request_log。本轮已有助手消息时改写为出站正文（带来源与实例标记），否则再插入一条。
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
			if !b.upsertOutboundAssistant(tr, text, rid) {
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

// upsertOutboundAssistant 把出站正文写回本轮助手消息。
// 返回 true 表示这条回复已经落库，调用方不要再插入。
func (b *Bot) upsertOutboundAssistant(tr *msgTrace, text, requestID string) bool {
	if b == nil || b.db == nil || tr == nil || tr.conversationID == nil {
		return false
	}
	if tr.assistantMsgID != nil && *tr.assistantMsgID != uuid.Nil {
		var existing model.Message
		err := b.db.Where("id = ? AND conversation_id = ? AND role = ?", *tr.assistantMsgID, *tr.conversationID, "assistant").
			First(&existing).Error
		if err != nil {
			if b.log != nil {
				b.log.Error("load outbound assistant message", zap.Error(err), zap.String("request_id", requestID))
			}
			return false
		}
		if existing.Content == text {
			return true
		}
		if err := b.db.Model(&existing).Update("content", text).Error; err != nil {
			if b.log != nil {
				b.log.Error("update outbound assistant message", zap.Error(err), zap.String("request_id", requestID))
			}
		}
		return true
	}
	var n int64
	_ = b.db.Model(&model.Message{}).
		Where("conversation_id = ? AND role = ? AND content = ?", *tr.conversationID, "assistant", text).
		Count(&n).Error
	return n > 0
}

func (b *Bot) sanitizeOutbound(text string) string {
	if !agent.IsToolEvasionReply(text) {
		return agent.MaskSchemaLeak(text)
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

type corpusSourceGroup struct {
	Name   string
	Titles []string
}

// buildCorpusSourceNote 根据本轮 RAG 命中生成「来源：语料库（1.标题 2.标题）」。
func (b *Bot) buildCorpusSourceNote(hits []rag.Hit) string {
	return formatCorpusSourceNote(b.corpusSourceGroups(hits))
}

func (b *Bot) corpusSourceGroups(hits []rag.Hit) []corpusSourceGroup {
	if len(hits) == 0 {
		return nil
	}
	corpusName, docTitle := b.loadHitSourceMeta(hits)
	type acc struct {
		name   string
		titles []string
		seen   map[string]struct{}
	}
	byCorpus := map[uuid.UUID]*acc{}
	order := make([]uuid.UUID, 0, 4)
	for _, h := range hits {
		cid := h.CorpusID
		a, ok := byCorpus[cid]
		if !ok {
			name := corpusName[cid]
			if name == "" {
				name = "语料库"
			}
			a = &acc{name: name, seen: map[string]struct{}{}}
			byCorpus[cid] = a
			order = append(order, cid)
		}
		title := strings.TrimSpace(docTitle[h.DocumentID])
		if title == "" {
			continue
		}
		if _, dup := a.seen[title]; dup {
			continue
		}
		a.seen[title] = struct{}{}
		if len(a.titles) >= 5 {
			continue
		}
		a.titles = append(a.titles, title)
	}
	out := make([]corpusSourceGroup, 0, len(order))
	for _, id := range order {
		a := byCorpus[id]
		if a == nil {
			continue
		}
		out = append(out, corpusSourceGroup{Name: a.name, Titles: a.titles})
	}
	return out
}

func (b *Bot) loadHitSourceMeta(hits []rag.Hit) (corpusName map[uuid.UUID]string, docTitle map[uuid.UUID]string) {
	corpusName = map[uuid.UUID]string{}
	docTitle = map[uuid.UUID]string{}
	seenC := map[uuid.UUID]struct{}{}
	seenD := map[uuid.UUID]struct{}{}
	corpusIDs := make([]uuid.UUID, 0, 4)
	docIDs := make([]uuid.UUID, 0, 8)
	for _, h := range hits {
		if h.CorpusID != uuid.Nil {
			if _, ok := seenC[h.CorpusID]; !ok {
				seenC[h.CorpusID] = struct{}{}
				corpusIDs = append(corpusIDs, h.CorpusID)
			}
		}
		if h.DocumentID != uuid.Nil {
			if _, ok := seenD[h.DocumentID]; !ok {
				seenD[h.DocumentID] = struct{}{}
				docIDs = append(docIDs, h.DocumentID)
			}
		}
	}
	if b == nil || b.db == nil {
		return corpusName, docTitle
	}
	if len(corpusIDs) > 0 {
		var rows []model.Corpus
		if err := b.db.Select("id", "name").Where("id IN ?", corpusIDs).Find(&rows).Error; err == nil {
			for _, r := range rows {
				corpusName[r.ID] = strings.TrimSpace(r.Name)
			}
		}
	}
	if len(docIDs) > 0 {
		var docs []model.Document
		if err := b.db.Select("id", "title", "source").Where("id IN ?", docIDs).Find(&docs).Error; err == nil {
			for _, d := range docs {
				docTitle[d.ID] = shortDocTitle(d.Title, d.Source)
			}
		}
	}
	return corpusName, docTitle
}

func formatCorpusSourceNote(groups []corpusSourceGroup) string {
	if len(groups) == 0 {
		return ""
	}
	merged := make([]corpusSourceGroup, 0, len(groups))
	idx := map[string]int{}
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = "语料库"
		}
		if i, ok := idx[name]; ok {
			seen := map[string]struct{}{}
			for _, t := range merged[i].Titles {
				seen[t] = struct{}{}
			}
			for _, t := range g.Titles {
				if t == "" {
					continue
				}
				if _, dup := seen[t]; dup {
					continue
				}
				if len(merged[i].Titles) >= 5 {
					break
				}
				seen[t] = struct{}{}
				merged[i].Titles = append(merged[i].Titles, t)
			}
			continue
		}
		idx[name] = len(merged)
		merged = append(merged, corpusSourceGroup{Name: name, Titles: append([]string(nil), g.Titles...)})
		if len(merged) >= 3 {
			break
		}
	}
	parts := make([]string, 0, len(merged))
	for _, g := range merged {
		if len(g.Titles) == 0 {
			parts = append(parts, g.Name)
			continue
		}
		parts = append(parts, g.Name+numberedTitles(g.Titles))
	}
	return "来源：" + strings.Join(parts, "；")
}

func numberedTitles(titles []string) string {
	var b strings.Builder
	b.WriteString("（")
	for i, t := range titles {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteByte('.')
		b.WriteString(t)
	}
	b.WriteString("）")
	return b.String()
}

func shortDocTitle(title, source string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		s = strings.TrimSpace(source)
	}
	s = strings.TrimSuffix(s, ".pdf")
	s = strings.TrimSuffix(s, ".PDF")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) > 24 {
		return string(runes[:24]) + "…"
	}
	return s
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
