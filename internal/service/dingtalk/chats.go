package dingtalk

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChatItem 控制台钉钉会话列表项。
type ChatItem struct {
	ID                   uuid.UUID     `json:"id"`
	ConversationID       string        `json:"conversation_id"`
	Title                string        `json:"title"`
	ConversationType     string        `json:"conversation_type"`
	IsGroup              bool          `json:"is_group"`
	LastSeenAt           time.Time     `json:"last_seen_at"`
	UpdatedAt            time.Time     `json:"updated_at"`
	PrefixPrompt         string        `json:"prefix_prompt"`
	HistoryLimit         int           `json:"history_limit"`
	Tools                []string      `json:"tools"`
	ReplyMode            string        `json:"reply_mode"`
	LLMProvider          string        `json:"llm_provider"`
	LLMModel             string        `json:"llm_model"`
	DictionaryDocumentID string        `json:"dictionary_document_id"`
	CorpusIDs            []uuid.UUID   `json:"corpus_ids"`
	Corpora              []CorpusBrief `json:"corpora"`
}

// ErrDocumentNotFound 数据字典所选语料文档不存在。
var ErrDocumentNotFound = errors.New("document not found")

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

// SetChatDictionary 保存该群/单聊的数据字典。documentID 为空表示清除。
func (b *Bot) SetChatDictionary(id uuid.UUID, documentID *uuid.UUID) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if documentID != nil {
		var doc model.Document
		err := b.db.Select("id").First(&doc, "id = ?", *documentID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrDocumentNotFound, documentID)
		}
		if err != nil {
			return nil, err
		}
	}
	stored := any(gorm.Expr("NULL"))
	if documentID != nil {
		stored = *documentID
	}
	if err := b.db.Model(&chat).Update("dictionary_document_id", stored).Error; err != nil {
		return nil, err
	}
	chat.DictionaryDocumentID = documentID
	item := toChatItem(chat, b.corporaForChat(id))
	return &item, nil
}

// SetChatPrefix 保存该群/单聊的前置提示词。空字符串表示清除。
func (b *Bot) SetChatPrefix(id uuid.UUID, prompt string) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	prompt = strings.TrimSpace(prompt)
	if len([]rune(prompt)) > 4000 {
		return nil, ErrPrefixTooLong
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := b.db.Model(&chat).Update("prefix_prompt", prompt).Error; err != nil {
		return nil, err
	}
	chat.PrefixPrompt = prompt
	item := toChatItem(chat, b.corporaForChat(id))
	return &item, nil
}

// SetChatHistoryLimit 保存该群带入大模型的历史消息条数。0 表示不带历史。
func (b *Bot) SetChatHistoryLimit(id uuid.UUID, n int) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	if n < 0 || n > MaxHistoryLimit {
		return nil, ErrHistoryLimit
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := b.db.Model(&chat).Update("history_limit", n).Error; err != nil {
		return nil, err
	}
	chat.HistoryLimit = n
	item := toChatItem(chat, b.corporaForChat(id))
	return &item, nil
}

// chatHistoryLimit 返回该钉钉会话的上下文条数。未配置时为 0，不回退到 llm.max_history。
func (b *Bot) chatHistoryLimit(conversationID string) *int {
	n := 0
	if b != nil && b.db != nil {
		cid := strings.TrimSpace(conversationID)
		if cid != "" {
			var row model.DingTalkChat
			err := b.db.Select("history_limit").Where("conversation_id = ?", cid).First(&row).Error
			if err == nil && row.HistoryLimit > 0 {
				n = row.HistoryLimit
			}
		}
	}
	return &n
}

func (b *Bot) chatDictionary(conversationID string) string {
	if b == nil || b.db == nil {
		return ""
	}
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return ""
	}
	var row model.DingTalkChat
	if err := b.db.Where("conversation_id = ?", cid).First(&row).Error; err != nil {
		return ""
	}
	if row.DictionaryDocumentID == nil || *row.DictionaryDocumentID == uuid.Nil {
		return ""
	}
	var doc model.Document
	if err := b.db.First(&doc, "id = ?", *row.DictionaryDocumentID).Error; err != nil {
		return ""
	}
	if strings.TrimSpace(doc.Content) != "" {
		return doc.Content
	}
	if b.corpus == nil {
		return ""
	}
	text, err := b.corpus.DocumentContent(&doc)
	if err != nil {
		return ""
	}
	return text
}

// chatPrefix 返回注入系统提示的包装文案，以及原文是否要求跳过查库。
// 第二个返回值只看库里的原文，不看包装句。
func (b *Bot) chatPrefix(conversationID string) (string, bool) {
	raw := b.rawChatPrefix(conversationID)
	return wrapPrefixPrompt(raw), agent.PrefixSkipsQuery(raw)
}

func (b *Bot) rawChatPrefix(conversationID string) string {
	if b == nil || b.db == nil {
		return ""
	}
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return ""
	}
	var row model.DingTalkChat
	err := b.db.Select("prefix_prompt").Where("conversation_id = ?", cid).First(&row).Error
	if err != nil {
		return ""
	}
	return row.PrefixPrompt
}

// ErrPrefixTooLong 群前置提示词超过 4000 字。
var ErrPrefixTooLong = errors.New("prefix prompt too long")

// ErrHistoryLimit 上下文条数不在 0 到 200。
var ErrHistoryLimit = errors.New("history limit out of range")

// MaxHistoryLimit 单个钉钉会话允许带入的历史消息条数上限。
const MaxHistoryLimit = 200

// ErrUnknownTool 群工具名不在已注册工具中。
var ErrUnknownTool = errors.New("unknown tool")

// ErrUnknownLLM 厂商不存在、未启用或未配置密钥。
var ErrUnknownLLM = errors.New("unknown llm provider")

// ErrInvalidLLMModel 厂商与模型必须同时填写，且模型 id 只含允许的字符。
var ErrInvalidLLMModel = errors.New("invalid llm model")

var llmModelID = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)

// SetChatLLM 保存该会话使用的厂商和模型。两者都空则恢复默认。
func (b *Bot) SetChatLLM(id uuid.UUID, provider, modelName string) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	provider, modelName, err := b.normalizeChatLLM(provider, modelName)
	if err != nil {
		return nil, err
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := b.db.Model(&chat).Updates(map[string]any{
		"llm_provider": provider,
		"llm_model":    modelName,
	}).Error; err != nil {
		return nil, err
	}
	chat.LLMProvider = provider
	chat.LLMModel = modelName
	item := toChatItem(chat, b.corporaForChat(id))
	return &item, nil
}

func (b *Bot) normalizeChatLLM(provider, modelName string) (string, string, error) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" && modelName == "" {
		return "", "", nil
	}
	if provider == "" || modelName == "" || !llmModelID.MatchString(modelName) {
		return "", "", ErrInvalidLLMModel
	}
	name, err := b.lookupLLMProvider(provider)
	if err != nil {
		return "", "", err
	}
	return name, modelName, nil
}

func (b *Bot) lookupLLMProvider(provider string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(provider))
	switch name {
	case "dashscope":
		name = "qwen"
	case "moonshot":
		name = "kimi"
	case "ark", "volcengine":
		name = "doubao"
	}
	if b == nil {
		return "", ErrUnknownLLM
	}
	pc, ok := b.llm.Providers[name]
	if !ok || !pc.ProviderEnabled() || strings.TrimSpace(pc.APIKey) == "" || strings.TrimSpace(pc.BaseURL) == "" {
		return "", ErrUnknownLLM
	}
	return name, nil
}

// chatLLM 返回该会话单独指定的厂商和模型。未同时配置时返回空，调用方走默认厂商。
func (b *Bot) chatLLM(conversationID string) (provider, modelName string) {
	if b == nil || b.db == nil {
		return "", ""
	}
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return "", ""
	}
	var row model.DingTalkChat
	err := b.db.Select("llm_provider", "llm_model").Where("conversation_id = ?", cid).First(&row).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) && b.log != nil {
			b.log.Warn("load dingtalk chat llm", zap.Error(err), zap.String("conversation_id", cid))
		}
		return "", ""
	}
	provider = strings.TrimSpace(row.LLMProvider)
	modelName = strings.TrimSpace(row.LLMModel)
	if provider == "" || modelName == "" {
		return "", ""
	}
	return provider, modelName
}

// SetChatTools 保存该群额外工具。空数组表示清除。执行时与 agent.default_tools 取并集。
func (b *Bot) SetChatTools(id uuid.UUID, names []string) (*ChatItem, error) {
	if b == nil || b.db == nil {
		return nil, fmt.Errorf("database not configured")
	}
	cleaned, err := b.normalizeChatTools(names)
	if err != nil {
		return nil, err
	}
	raw, err := encodeTools(cleaned)
	if err != nil {
		return nil, err
	}
	var chat model.DingTalkChat
	if err := b.db.First(&chat, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := b.db.Model(&chat).Update("tools", raw).Error; err != nil {
		return nil, err
	}
	chat.Tools = raw
	item := toChatItem(chat, b.corporaForChat(id))
	return &item, nil
}

func (b *Bot) normalizeChatTools(names []string) ([]string, error) {
	if b == nil || b.agent == nil {
		return nil, fmt.Errorf("agent not configured")
	}
	allowed := make(map[string]struct{}, 8)
	for _, t := range b.agent.ListTools() {
		allowed[t.Name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		if _, ok := allowed[n]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownTool, n)
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func (b *Bot) chatTools(conversationID string) []string {
	if b == nil || b.db == nil {
		return nil
	}
	cid := strings.TrimSpace(conversationID)
	if cid == "" {
		return nil
	}
	var row model.DingTalkChat
	err := b.db.Select("tools").Where("conversation_id = ?", cid).First(&row).Error
	if err != nil {
		return nil
	}
	return decodeTools(row.Tools)
}

func encodeTools(names []string) (string, error) {
	if len(names) == 0 {
		return "", nil
	}
	b, err := json.Marshal(names)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeTools(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// wrapPrefixPrompt 把群规则放在系统提示最前。
// 包装句说明：先查库的规则必须先调工具；只有原文写明不要查库时才只追问。
func wrapPrefixPrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return ""
	}
	return "【本群规则，优先于后续工具调用约定】\n" + p + "\n" +
		"规则要求先查库时必须先调用工具，不要凭历史结果或猜测作答。需要用户补充信息时，放在工具返回之后。只有规则写明不要查库时，本轮才只回复追问、不调用工具。"
}

func toChatItem(row model.DingTalkChat, corpora []model.Corpus) ChatItem {
	ids := make([]uuid.UUID, 0, len(corpora))
	briefs := make([]CorpusBrief, 0, len(corpora))
	for _, c := range corpora {
		ids = append(ids, c.ID)
		briefs = append(briefs, CorpusBrief{ID: c.ID, Name: c.Name})
	}
	return ChatItem{
		ID:                   row.ID,
		ConversationID:       row.ConversationID,
		Title:                row.Title,
		ConversationType:     row.ConversationType,
		IsGroup:              isGroup(row.ConversationType),
		LastSeenAt:           row.LastSeenAt,
		UpdatedAt:            row.UpdatedAt,
		PrefixPrompt:         row.PrefixPrompt,
		HistoryLimit:         row.HistoryLimit,
		Tools:                decodeTools(row.Tools),
		ReplyMode:            row.ReplyMode,
		LLMProvider:          row.LLMProvider,
		LLMModel:             row.LLMModel,
		DictionaryDocumentID: dictionaryDocumentID(row.DictionaryDocumentID),
		CorpusIDs:            ids,
		Corpora:              briefs,
	}
}

func dictionaryDocumentID(id *uuid.UUID) string {
	if id == nil || *id == uuid.Nil {
		return ""
	}
	return id.String()
}
