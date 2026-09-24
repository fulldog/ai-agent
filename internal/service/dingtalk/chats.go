package dingtalk

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChatItem 控制台钉钉会话列表项。
type ChatItem struct {
	ID               uuid.UUID     `json:"id"`
	ConversationID   string        `json:"conversation_id"`
	Title            string        `json:"title"`
	ConversationType string        `json:"conversation_type"`
	IsGroup          bool          `json:"is_group"`
	LastSeenAt       time.Time     `json:"last_seen_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	CorpusIDs        []uuid.UUID   `json:"corpus_ids"`
	Corpora          []CorpusBrief `json:"corpora"`
}

// CorpusBrief 绑定语料库摘要。
type CorpusBrief struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func chatDisplayTitle(data *botCallback) string {
	if data == nil {
		return ""
	}
	title := strings.TrimSpace(data.ConversationTitle)
	if title != "" {
		return title
	}
	if isGroup(data.ConversationType) {
		return "钉钉群聊"
	}
	return "钉钉单聊"
}

func (b *Bot) upsertChat(data *botCallback) {
	if b == nil || b.db == nil || data == nil {
		return
	}
	cid := strings.TrimSpace(data.ConversationID)
	if cid == "" {
		return
	}
	now := time.Now()
	var row model.DingTalkChat
	err := b.db.Where("conversation_id = ?", cid).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = model.DingTalkChat{
			ConversationID:   cid,
			Title:            chatDisplayTitle(data),
			ConversationType: strings.TrimSpace(data.ConversationType),
			LastSeenAt:       now,
		}
		if cerr := b.db.Create(&row).Error; cerr != nil {
			b.log.Warn("upsert dingtalk chat", zap.Error(cerr), zap.String("conversation_id", cid))
		}
		return
	}
	if err != nil {
		b.log.Warn("load dingtalk chat", zap.Error(err), zap.String("conversation_id", cid))
		return
	}
	updates := map[string]any{
		"conversation_type": strings.TrimSpace(data.ConversationType),
		"last_seen_at":      now,
	}
	if t := strings.TrimSpace(data.ConversationTitle); t != "" {
		updates["title"] = t
	} else if strings.TrimSpace(row.Title) == "" {
		updates["title"] = chatDisplayTitle(data)
	}
	if uerr := b.db.Model(&row).Updates(updates).Error; uerr != nil {
		b.log.Warn("update dingtalk chat", zap.Error(uerr), zap.String("conversation_id", cid))
	}
}

func (b *Bot) boundCorpora(conversationID string) []model.Corpus {
	if b == nil || b.db == nil {
		return nil
	}
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return nil
	}
	var chat model.DingTalkChat
	err := b.db.Where("conversation_id = ?", cid).First(&chat).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			b.log.Warn("load dingtalk chat corpora", zap.Error(err), zap.String("conversation_id", cid))
		}
		return nil
	}
	return b.corporaForChat(chat.ID)
}

func (b *Bot) corporaForChat(chatID uuid.UUID) []model.Corpus {
	if b == nil || b.db == nil || chatID == uuid.Nil {
		return nil
	}
	var links []model.DingTalkChatCorpus
	if err := b.db.Where("chat_id = ?", chatID).Find(&links).Error; err != nil {
		b.log.Warn("list dingtalk chat corpora", zap.Error(err), zap.String("chat_id", chatID.String()))
		return nil
	}
	if len(links) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.CorpusID)
	}
	var rows []model.Corpus
	if err := b.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		b.log.Warn("load bound corpora", zap.Error(err), zap.String("chat_id", chatID.String()))
		return nil
	}
	byID := make(map[uuid.UUID]model.Corpus, len(rows))
	for _, c := range rows {
		byID[c.ID] = c
	}
	out := make([]model.Corpus, 0, len(ids))
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// backfillChatsFromConversations 从已有钉钉会话回填群档案（升级前消息不会自动写入 dingtalk_chats）。
func (b *Bot) backfillChatsFromConversations() {
	if b == nil || b.db == nil {
		return
	}
	type convRow struct {
		ChannelSessionID string
		Title            string
		UpdatedAt        time.Time
	}
	var convs []convRow
	err := b.db.Model(&model.Conversation{}).
		Select("channel_session_id, title, updated_at").
		Where("channel = ? AND channel_session_id <> '' AND deleted_at IS NULL", channelDingTalk).
		Order("updated_at desc").
		Find(&convs).Error
	if err != nil {
		b.log.Warn("backfill dingtalk chats: list conversations", zap.Error(err))
		return
	}
	seen := map[string]struct{}{}
	for _, c := range convs {
		cid := strings.TrimSpace(c.ChannelSessionID)
		if cid == "" {
			continue
		}
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		var n int64
		if err := b.db.Model(&model.DingTalkChat{}).Where("conversation_id = ?", cid).Count(&n).Error; err != nil {
			b.log.Warn("backfill dingtalk chats: count", zap.Error(err), zap.String("conversation_id", cid))
			continue
		}
		if n > 0 {
			continue
		}
		title, convType := titleFromLegacyConversation(c.Title)
		row := model.DingTalkChat{
			ConversationID:   cid,
			Title:            title,
			ConversationType: convType,
			LastSeenAt:       c.UpdatedAt,
		}
		if cerr := b.db.Create(&row).Error; cerr != nil {
			b.log.Warn("backfill dingtalk chat", zap.Error(cerr), zap.String("conversation_id", cid))
		}
	}
}

func titleFromLegacyConversation(raw string) (title, conversationType string) {
	t := strings.TrimSpace(raw)
	if i := strings.LastIndex(t, " · "); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	switch t {
	case "钉钉单聊":
		return t, "1"
	case "钉钉群聊", "":
		if t == "" {
			t = "钉钉群聊"
		}
		return t, "2"
	default:
		return t, "2"
	}
}

func (b *Bot) ListChats() ([]ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	b.backfillChatsFromConversations()
	var rows []model.DingTalkChat
	if err := b.db.Order("last_seen_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ChatItem{}, nil
	}
	chatIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		chatIDs = append(chatIDs, row.ID)
	}
	var links []model.DingTalkChatCorpus
	if err := b.db.Where("chat_id IN ?", chatIDs).Find(&links).Error; err != nil {
		return nil, err
	}
	corpusIDSet := make(map[uuid.UUID]struct{})
	linksByChat := make(map[uuid.UUID][]uuid.UUID)
	for _, l := range links {
		linksByChat[l.ChatID] = append(linksByChat[l.ChatID], l.CorpusID)
		corpusIDSet[l.CorpusID] = struct{}{}
	}
	byCorpus := map[uuid.UUID]model.Corpus{}
	if len(corpusIDSet) > 0 {
		ids := make([]uuid.UUID, 0, len(corpusIDSet))
		for id := range corpusIDSet {
			ids = append(ids, id)
		}
		var corpora []model.Corpus
		if err := b.db.Where("id IN ?", ids).Find(&corpora).Error; err != nil {
			return nil, err
		}
		for _, c := range corpora {
			byCorpus[c.ID] = c
		}
	}
	out := make([]ChatItem, 0, len(rows))
	for _, row := range rows {
		var bound []model.Corpus
		for _, cid := range linksByChat[row.ID] {
			if c, ok := byCorpus[cid]; ok {
				bound = append(bound, c)
			}
		}
		out = append(out, toChatItem(row, bound))
	}
	return out, nil
}

func (b *Bot) SetChatCorpora(id uuid.UUID, corpusIDs []uuid.UUID) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	bound := make([]model.Corpus, 0, len(corpusIDs))
	seen := map[uuid.UUID]struct{}{}
	for _, cid := range corpusIDs {
		if cid == uuid.Nil {
			return nil, fmt.Errorf("invalid corpus_id")
		}
		if _, ok := seen[cid]; ok {
			continue
		}
		seen[cid] = struct{}{}
		var c model.Corpus
		if err := b.db.First(&c, "id = ?", cid).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("corpus not found: %s", cid)
			}
			return nil, err
		}
		bound = append(bound, c)
	}
	err := b.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chat_id = ?", id).Delete(&model.DingTalkChatCorpus{}).Error; err != nil {
			return err
		}
		for _, c := range bound {
			if err := tx.Create(&model.DingTalkChatCorpus{ChatID: id, CorpusID: c.ID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	item := toChatItem(chat, bound)
	return &item, nil
}

func toChatItem(row model.DingTalkChat, corpora []model.Corpus) ChatItem {
	ids := make([]uuid.UUID, 0, len(corpora))
	briefs := make([]CorpusBrief, 0, len(corpora))
	for _, c := range corpora {
		ids = append(ids, c.ID)
		briefs = append(briefs, CorpusBrief{ID: c.ID, Name: c.Name})
	}
	return ChatItem{
		ID:               row.ID,
		ConversationID:   row.ConversationID,
		Title:            row.Title,
		ConversationType: row.ConversationType,
		IsGroup:          isGroup(row.ConversationType),
		LastSeenAt:       row.LastSeenAt,
		UpdatedAt:        row.UpdatedAt,
		CorpusIDs:        ids,
		Corpora:          briefs,
	}
}
