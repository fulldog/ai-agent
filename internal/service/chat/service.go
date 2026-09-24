package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent/tools"
	"github.com/webapp/go-app/ai-agent/internal/service/dbconn"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db       *gorm.DB
	cfg      *config.Config
	pool     *llm.Pool
	rag      *rag.Service
	registry *tools.Registry
	llmLog   *zap.Logger // 完整 prompt/回复 → logs/llm-*.log
}

func New(db *gorm.DB, cfg *config.Config, pool *llm.Pool, ragSvc *rag.Service, llmLog *zap.Logger, bizDB *dbconn.Client) *Service {
	if llmLog == nil {
		llmLog = zap.NewNop()
	}
	reg := tools.Default()
	if bizDB != nil {
		reg = tools.WithDBConn(bizDB)
	}
	return &Service{db: db, cfg: cfg, pool: pool, rag: ragSvc, registry: reg, llmLog: llmLog}
}

type CreateConversationInput struct {
	UID              string
	Title            string
	SystemPrompt     string
	CorpusID         *uuid.UUID
	Channel          string
	ChannelSessionID string
}

func (s *Service) CreateConversation(in CreateConversationInput) (*model.Conversation, error) {
	uid := strings.TrimSpace(in.UID)
	if uid == "" {
		return nil, fmt.Errorf("uid required")
	}
	c := &model.Conversation{
		UID:              uid,
		Title:            in.Title,
		SystemPrompt:     in.SystemPrompt,
		CorpusID:         in.CorpusID,
		Channel:          strings.TrimSpace(in.Channel),
		ChannelSessionID: strings.TrimSpace(in.ChannelSessionID),
	}
	if c.Title == "" {
		c.Title = "conversation"
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

// FindOrCreateByChannel 按 uid + 通道 + 通道会话 ID 复用会话（钉钉群/单聊隔离）。
func (s *Service) FindOrCreateByChannel(in CreateConversationInput) (*model.Conversation, error) {
	uid := strings.TrimSpace(in.UID)
	ch := strings.TrimSpace(in.Channel)
	sid := strings.TrimSpace(in.ChannelSessionID)
	if uid == "" {
		return nil, fmt.Errorf("uid required")
	}
	if ch == "" || sid == "" {
		return nil, fmt.Errorf("channel and channel_session_id required")
	}
	c, err := s.latestByChannel(uid, ch, sid)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		updates := map[string]any{}
		if in.Title != "" && c.Title != in.Title {
			updates["title"] = in.Title
		}
		if in.CorpusID != nil && (c.CorpusID == nil || *c.CorpusID != *in.CorpusID) {
			updates["corpus_id"] = *in.CorpusID
			c.CorpusID = in.CorpusID
		}
		if len(updates) > 0 {
			_ = s.db.Model(c).Updates(updates).Error
		}
		return c, nil
	}
	return s.CreateConversation(in)
}

// latestByChannel 取 uid+通道+通道会话 ID 下最新的未删除会话。
func (s *Service) latestByChannel(uid, channel, sessionID string) (*model.Conversation, error) {
	var c model.Conversation
	err := s.db.Where("uid = ? AND channel = ? AND channel_session_id = ?", uid, channel, sessionID).
		Order("created_at DESC").
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByChannel 按 uid + 通道 + 通道会话 ID 查找已有会话；不存在时返回 nil, nil。
func (s *Service) GetByChannel(uid, channel, sessionID string) (*model.Conversation, error) {
	uid = strings.TrimSpace(uid)
	ch := strings.TrimSpace(channel)
	sid := strings.TrimSpace(sessionID)
	if uid == "" || ch == "" || sid == "" {
		return nil, nil
	}
	if s == nil || s.db == nil {
		return nil, nil
	}
	c, err := s.latestByChannel(uid, ch, sid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) HasMessages(conversationID uuid.UUID) (bool, error) {
	if s == nil || s.db == nil || conversationID == uuid.Nil {
		return false, nil
	}
	var n int64
	err := s.db.Model(&model.Message{}).Where("conversation_id = ?", conversationID).Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *Service) ListConversations(uid string, limit, offset int) ([]model.Conversation, error) {
	rows, _, err := s.QueryConversations(uid, false, false, limit, offset)
	return rows, err
}

// QueryConversations 分页列出会话。all=true 时列出全库（可选 uid 过滤）；否则必须带 uid。
// includeDeleted=true 时含软删行（会话历史）；默认排除，供续聊侧栏使用。
func (s *Service) QueryConversations(uid string, all, includeDeleted bool, limit, offset int) ([]model.Conversation, int64, error) {
	uid = strings.TrimSpace(uid)
	if !all && uid == "" {
		return nil, 0, fmt.Errorf("uid required")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q := s.db.Model(&model.Conversation{})
	if includeDeleted {
		q = q.Unscoped()
	}
	if uid != "" {
		q = q.Where("uid = ?", uid)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.Conversation
	err := q.Order("created_at desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (s *Service) GetConversationByID(id uuid.UUID) (*model.Conversation, error) {
	return s.lookupConversation(id, "", false)
}

// GetConversationByIDIncludingDeleted 按 id 读会话，含已软删（历史详情/消息）。
func (s *Service) GetConversationByIDIncludingDeleted(id uuid.UUID) (*model.Conversation, error) {
	return s.lookupConversation(id, "", true)
}

func (s *Service) DeleteConversationByID(id uuid.UUID) error {
	return s.deleteConversation(id, "")
}

func (s *Service) GetConversation(id uuid.UUID, uid string) (*model.Conversation, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return s.lookupConversation(id, uid, false)
}

// GetConversationIncludingDeleted 按 id+uid 读会话，含已软删（历史详情/消息）。
func (s *Service) GetConversationIncludingDeleted(id uuid.UUID, uid string) (*model.Conversation, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return s.lookupConversation(id, uid, true)
}

func (s *Service) lookupConversation(id uuid.UUID, uid string, includeDeleted bool) (*model.Conversation, error) {
	q := s.db
	if includeDeleted {
		q = q.Unscoped()
	}
	var c model.Conversation
	var err error
	if uid != "" {
		err = q.First(&c, "id = ? AND uid = ?", id, uid).Error
	} else {
		err = q.First(&c, "id = ?", id).Error
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) DeleteConversation(id uuid.UUID, uid string) error {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return gorm.ErrRecordNotFound
	}
	return s.deleteConversation(id, uid)
}

// deleteConversation 软删会话，保留 messages 供历史查看。uid 非空时校验归属。
func (s *Service) deleteConversation(id uuid.UUID, uid string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("database not configured")
	}
	q := s.db.Where("id = ?", id)
	if uid != "" {
		q = q.Where("uid = ?", uid)
	}
	res := q.Delete(&model.Conversation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) ListMessagesByID(conversationID uuid.UUID, limit int) ([]model.Message, error) {
	if _, err := s.GetConversationByIDIncludingDeleted(conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var rows []model.Message
	err := s.db.Where("conversation_id = ?", conversationID).Order("created_at asc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) ListMessages(conversationID uuid.UUID, uid string, limit int) ([]model.Message, error) {
	if _, err := s.GetConversationIncludingDeleted(conversationID, uid); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var rows []model.Message
	err := s.db.Where("conversation_id = ?", conversationID).Order("created_at asc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) maxHistory() int {
	if s != nil && s.cfg != nil && s.cfg.LLM.MaxHistory > 0 {
		return s.cfg.LLM.MaxHistory
	}
	return 10
}

// wantRAG 是否对本轮对话做向量检索注入。请求显式传 rag 时尊重开关；否则用 chat.rag_enabled（默认开）。
func (s *Service) wantRAG(in CompleteInput) bool {
	if in.RAGExplicit {
		return in.RAGEnabled
	}
	if s != nil && s.cfg != nil {
		return s.cfg.Chat.IsRAGEnabled()
	}
	return true
}

// RecentMessages 取最近 limit 条，再按时间正序返回（钉钉 RAG 追问扩写、LLM 上下文共用）。
func (s *Service) RecentMessages(conversationID uuid.UUID, limit int) ([]model.Message, error) {
	return s.listRecentMessages(conversationID, limit)
}

// listRecentMessages 取最近 limit 条，再按时间正序返回，供 LLM 上下文使用。
func (s *Service) listRecentMessages(conversationID uuid.UUID, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows []model.Message
	err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at desc").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	slices.Reverse(rows)
	return rows, nil
}

type CompleteInput struct {
	ConversationID uuid.UUID
	UID            string
	Admin          bool
	Message        string
	Provider       string
	Model          string
	Temperature    float64
	MaxTokens      int
	RAGEnabled     bool
	// RAGExplicit 为 true 时尊重 RAGEnabled；为 false 时用 chat.rag_enabled 默认（缺省开启）。
	RAGExplicit   bool
	CorpusID      *uuid.UUID
	CorpusIDs     []uuid.UUID
	RAGHits       []rag.Hit
	TopK          int
	EnableSearch  bool // 无语料命中且用户强制联网时，通义 enable_search
	RequestID     string
	LogLLMRequest func(provider, model string, messages []llm.Message)
}

type CompleteResult struct {
	ConversationID   uuid.UUID
	MessageID        uuid.UUID
	Content          string
	PromptTokens     int
	CompletionTokens int
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (*CompleteResult, error) {
	conv, _, llmMsgs, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	if in.LogLLMRequest != nil {
		in.LogLLMRequest(providerName, modelName, llmMsgs)
	}
	call := callContext{conv: conv, client: client, provider: providerName, model: modelName}
	resp, err := s.toolLoop(ctx, in, call, llmMsgs, nil)
	if llm.IsInspectionFailed(err) && (s.wantRAG(in) || len(in.RAGHits) > 0) {
		s.llmLog.Warn("retry llm without rag after content inspection")
		retry := in
		retry.RAGEnabled = false
		retry.RAGExplicit = true
		retry.RAGHits = nil
		if _, _, retryMsgs, perr := s.prepare(ctx, retry); perr == nil {
			resp, err = s.toolLoop(ctx, retry, call, retryMsgs, nil)
		}
	}
	if err != nil {
		return nil, err
	}
	userMsg := model.Message{ConversationID: conv.ID, Role: "user", Content: in.Message}
	if err := s.db.Create(&userMsg).Error; err != nil {
		return nil, err
	}
	pt, ct := resp.PromptTokens, resp.CompletionTokens
	asst := model.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         resp.Content,
		TokenPrompt:     &pt,
		TokenCompletion: &ct,
	}
	if err := s.db.Create(&asst).Error; err != nil {
		return nil, err
	}
	return &CompleteResult{
		ConversationID:   conv.ID,
		MessageID:        asst.ID,
		Content:          resp.Content,
		PromptTokens:     pt,
		CompletionTokens: ct,
	}, nil
}

func (s *Service) CompleteStream(ctx context.Context, in CompleteInput, onDelta func(string) error) (*CompleteResult, error) {
	conv, _, llmMsgs, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	if in.LogLLMRequest != nil {
		in.LogLLMRequest(providerName, modelName, llmMsgs)
	}
	userMsg := model.Message{ConversationID: conv.ID, Role: "user", Content: in.Message}
	if err := s.db.Create(&userMsg).Error; err != nil {
		return nil, err
	}
	if onDelta == nil {
		onDelta = func(string) error { return nil }
	}
	call := callContext{conv: conv, client: client, provider: providerName, model: modelName}
	resp, err := s.toolLoop(ctx, in, call, llmMsgs, onDelta)
	if llm.IsInspectionFailed(err) && (s.wantRAG(in) || len(in.RAGHits) > 0) {
		s.llmLog.Warn("retry llm stream without rag after content inspection")
		retry := in
		retry.RAGEnabled = false
		retry.RAGExplicit = true
		retry.RAGHits = nil
		if _, _, retryMsgs, perr := s.prepare(ctx, retry); perr == nil {
			resp, err = s.toolLoop(ctx, retry, call, retryMsgs, onDelta)
		}
	}
	if err != nil {
		return nil, err
	}
	pt, ct := resp.PromptTokens, resp.CompletionTokens
	content := resp.Content
	asst := model.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         content,
		TokenPrompt:     &pt,
		TokenCompletion: &ct,
	}
	if err := s.db.Create(&asst).Error; err != nil {
		return nil, err
	}
	return &CompleteResult{
		ConversationID:   conv.ID,
		MessageID:        asst.ID,
		Content:          content,
		PromptTokens:     pt,
		CompletionTokens: ct,
	}, nil
}

func chatRequest(in CompleteInput, providerName, modelName string, msgs []llm.Message) llm.ChatRequest {
	return llm.ChatRequest{
		Model:        modelName,
		Messages:     msgs,
		Temperature:  in.Temperature,
		MaxTokens:    in.MaxTokens,
		EnableSearch: in.EnableSearch && strings.EqualFold(providerName, "qwen"),
	}
}

func (s *Service) prepare(ctx context.Context, in CompleteInput) (*model.Conversation, []model.Message, []llm.Message, error) {
	var conv *model.Conversation
	var err error
	if in.Admin {
		conv, err = s.GetConversationByID(in.ConversationID)
	} else {
		conv, err = s.GetConversation(in.ConversationID, in.UID)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	history, err := s.listRecentMessages(conv.ID, s.maxHistory())
	if err != nil {
		return nil, nil, nil, err
	}
	var llmMsgs []llm.Message
	corpusID := in.CorpusID
	if corpusID == nil {
		corpusID = conv.CorpusID
	}
	hits := in.RAGHits
	if s.wantRAG(in) && len(hits) == 0 && s.rag != nil {
		topK := in.TopK
		if topK <= 0 {
			topK = s.cfg.RAG.TopK
		}
		var found []rag.Hit
		var rerr error
		switch {
		case len(in.CorpusIDs) > 0:
			found, rerr = s.rag.SearchInCorpora(ctx, in.CorpusIDs, in.Message, topK)
		case corpusID != nil:
			found, rerr = s.rag.Search(ctx, *corpusID, in.Message, topK)
		default:
			found, rerr = s.rag.SearchInCorpora(ctx, nil, in.Message, topK)
		}
		if rerr == nil {
			hits = found
		}
	}
	style := s.replyStyle()
	if tp := toolSystemPrompt(s.toolSpecs()); tp != "" {
		style += "\n\n" + tp
	}
	system := mergeSystem(style, conv.SystemPrompt, ragContext(hits))
	if system != "" {
		llmMsgs = append(llmMsgs, llm.Message{Role: "system", Content: system})
	}
	for _, m := range history {
		if m.Role == "system" {
			continue
		}
		llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: in.Message})
	return conv, history, llmMsgs, nil
}

// 单次直传分析时，文件正文最大 rune 数（超出截断，避免撑爆上下文）。
const defaultAnalyzeMaxRunes = 80000

// AnalyzeInput 上传文件/正文，直接交给大模型分析（不入库语料库）。
type AnalyzeInput struct {
	ConversationID *uuid.UUID
	UID            string
	Admin          bool
	Message        string
	Fields         []string
	ResponseJSON   bool
	FileName       string
	FileText       string // 文本模式：已抽取正文
	RemoteFileID   string // 通义等：fileid:// 模式
	FileMode       string // text | file_id
	ContentHash    string
	ExtractionID   *uuid.UUID
	CacheHit       bool
	ExtractBackend string
	Provider       string
	Model          string
	ChatModelHint  string // 如 qwen-long
	Temperature    float64
	TempSet        bool
	MaxTokens      int
	MaxFileRunes   int
	RequestID      string
	Stream         bool
}

type AnalyzeResult struct {
	ConversationID   *uuid.UUID
	MessageID        *uuid.UUID
	Content          string
	Data             map[string]any
	FileName         string
	FileChars        int
	Truncated        bool
	ContentHash      string
	ExtractionID     *uuid.UUID
	CacheHit         bool
	ExtractBackend   string
	PromptTokens     int
	CompletionTokens int
}

func (s *Service) Analyze(ctx context.Context, in AnalyzeInput, onDelta func(string) error) (*AnalyzeResult, error) {
	wantJSON := in.ResponseJSON || len(in.Fields) > 0
	if strings.TrimSpace(in.Message) == "" && len(in.Fields) == 0 {
		return nil, fmt.Errorf("请提供 message（自定义问题）或 fields（要抽取的字段列表）")
	}
	if in.ConversationID != nil {
		var err error
		if in.Admin {
			_, err = s.GetConversationByID(*in.ConversationID)
		} else {
			_, err = s.GetConversation(*in.ConversationID, in.UID)
		}
		if err != nil {
			return nil, fmt.Errorf("conversation not found")
		}
	}
	mode := strings.ToLower(strings.TrimSpace(in.FileMode))
	if mode == "" {
		if in.RemoteFileID != "" {
			mode = "file_id"
		} else {
			mode = "text"
		}
	}
	if mode == "text" && strings.TrimSpace(in.FileText) == "" {
		return nil, fmt.Errorf("文件内容为空，无法分析（请确认 PDF 有文字层，或已配置 OCR）")
	}
	if mode == "file_id" && strings.TrimSpace(in.RemoteFileID) == "" {
		return nil, fmt.Errorf("缺少云端文件 ID，无法分析")
	}

	maxRunes := in.MaxFileRunes
	if maxRunes <= 0 {
		maxRunes = defaultAnalyzeMaxRunes
	}
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" {
		fileName = "upload"
	}

	var fileText string
	var truncated bool
	var userPrompt string
	if mode == "file_id" {
		userPrompt = buildAnalyzeUserPrompt(in, fileName, "", false, maxRunes, wantJSON)
		userPrompt = strings.Replace(userPrompt, "\n【文件内容】\n", "\n【说明】正文见系统已挂载的文件，请直接依据该文件作答。\n", 1)
	} else {
		fileText, truncated = truncateRunes(in.FileText, maxRunes)
		userPrompt = buildAnalyzeUserPrompt(in, fileName, fileText, truncated, maxRunes, wantJSON)
	}

	systemPrompt := "你是文档信息抽取助手。只依据用户提供的文件内容作答，不要编造。文件中找不到的字段填 null。"
	if wantJSON {
		systemPrompt = analyzeJSONSystemPrompt
	}

	provider := in.Provider
	modelNameReq := in.Model
	if mode == "file_id" && strings.TrimSpace(in.ChatModelHint) != "" && strings.TrimSpace(modelNameReq) == "" {
		modelNameReq = in.ChatModelHint
	}
	client, providerName, modelName, err := s.pool.Resolve(provider, modelNameReq)
	if err != nil {
		return nil, err
	}
	if mode == "file_id" && providerName == "qwen" {
		// fileid:// 需长文档模型
		if !strings.Contains(strings.ToLower(modelName), "long") && !strings.Contains(strings.ToLower(modelName), "doc") {
			modelName = "qwen-long"
		}
	}

	temp := in.Temperature
	if !in.TempSet && wantJSON {
		temp = 0
	}

	var msgs []llm.Message
	if mode == "file_id" {
		msgs = []llm.Message{
			{Role: "system", Content: "fileid://" + strings.TrimSpace(in.RemoteFileID)},
			{Role: "user", Content: systemPrompt + "\n\n" + userPrompt},
		}
	} else {
		msgs = []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	}

	req := llm.ChatRequest{
		Model: modelName, Messages: msgs,
		Temperature: temp, MaxTokens: in.MaxTokens,
	}
	if wantJSON && !in.Stream {
		req.ResponseFormat = "json_object"
	}

	start := time.Now()
	var resp *llm.ChatResponse
	if in.Stream {
		resp, err = client.ChatStream(ctx, req, func(ev llm.StreamEvent) error {
			if ev.Content != "" && onDelta != nil {
				return onDelta(ev.Content)
			}
			return nil
		})
	} else {
		resp, err = client.Chat(ctx, req)
		if err != nil && req.ResponseFormat != "" && looksLikeResponseFormatError(err) {
			req.ResponseFormat = ""
			resp, err = client.Chat(ctx, req)
		}
	}

	status := "ok"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	pt, ct := 0, 0
	content := ""
	if resp != nil {
		pt, ct = resp.PromptTokens, resp.CompletionTokens
		content = resp.Content
	}

	var convID *uuid.UUID
	if in.ConversationID != nil {
		convID = in.ConversationID
	}
	chars := len([]rune(fileText))
	if mode == "file_id" {
		chars = 0
	}
	finishReason := ""
	if resp != nil {
		finishReason = resp.FinishReason
	}
	llmog.Save(s.db, s.llmLog, &model.LLMCallLog{
		RequestID: in.RequestID, ConversationID: convID,
		Provider: providerName, Model: modelName, Stream: in.Stream, Status: status,
		PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
		RequestSummary: fmt.Sprintf("analyze file=%s mode=%s chars=%d truncated=%v json=%v fields=%d",
			fileName, mode, chars, truncated, wantJSON, len(in.Fields)),
		ErrorMessage: errMsg,
	}, &llmog.Payload{Messages: msgs, Response: content, FinishReason: finishReason})
	if err != nil {
		return nil, err
	}

	out := &AnalyzeResult{
		Content: content, FileName: fileName, FileChars: chars,
		Truncated: truncated, PromptTokens: pt, CompletionTokens: ct,
		ContentHash: in.ContentHash, CacheHit: in.CacheHit, ExtractionID: in.ExtractionID,
		ExtractBackend: in.ExtractBackend,
	}
	if wantJSON {
		data, perr := parseJSONObject(content)
		if perr != nil {
			return nil, fmt.Errorf("模型未返回合法 JSON: %w；原文: %s", perr, truncateForErr(content, 500))
		}
		if len(in.Fields) > 0 {
			data = alignFields(data, in.Fields)
		}
		out.Data = data
	}

	if in.ConversationID != nil && s.db != nil {
		q := in.Message
		if q == "" && len(in.Fields) > 0 {
			q = "抽取字段: " + strings.Join(in.Fields, "、")
		}
		userContent := fmt.Sprintf("[文件分析] %s\n文件: %s (%d字)%s",
			q, fileName, chars, map[bool]string{true: " [截断]", false: ""}[truncated])
		if mode == "file_id" {
			userContent = fmt.Sprintf("[文件分析] %s\n文件: %s (file_id=%s)", q, fileName, in.RemoteFileID)
		}
		_ = s.db.Create(&model.Message{ConversationID: *in.ConversationID, Role: "user", Content: userContent}).Error
		asst := model.Message{ConversationID: *in.ConversationID, Role: "assistant", Content: content, TokenPrompt: &pt, TokenCompletion: &ct}
		if err := s.db.Create(&asst).Error; err == nil {
			id := asst.ID
			out.MessageID = &id
			out.ConversationID = in.ConversationID
		}
	}
	return out, nil
}

const analyzeJSONSystemPrompt = "你是文档信息抽取助手。只依据「文件内容」抽取信息，严禁编造或推测。" +
	"必须只输出一个合法 JSON 对象（key-value），不要 Markdown，不要代码块，不要额外说明。" +
	"找不到、看不清、填空括号【】〖〗内为空/乱码/仅残留括号时，该字段值必须为 null。" +
	"日期/期限若月或日任一端缺失、空白或不可辨认，整个字段为 null，禁止用其它条款（如「每月5日」）补全。" +
	"同一条款若 OCR 出现互相矛盾的写法，取清晰可读的；仍不确定则 null。"

func buildAnalyzeUserPrompt(in AnalyzeInput, fileName, fileText string, truncated bool, maxRunes int, wantJSON bool) string {
	var b strings.Builder
	b.WriteString("【任务】根据「文件内容」完成下列要求。\n")
	b.WriteString("【硬性规则】只抄写文件中明确写出的内容；填空未填、乱码、缺月/缺日 → 对应字段填 null；禁止猜测补全。\n\n")
	if len(in.Fields) > 0 {
		b.WriteString("请从文件中抽取以下字段，输出一个 JSON 对象，key 必须与下列字段名完全一致：\n")
		for _, f := range in.Fields {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(f)
			b.WriteByte('\n')
		}
		b.WriteString("\n示例格式（仅示例结构）：\n")
		example := make(map[string]any, len(in.Fields))
		for _, f := range in.Fields {
			f = strings.TrimSpace(f)
			if f != "" {
				example[f] = nil
			}
		}
		if raw, err := json.Marshal(example); err == nil {
			b.Write(raw)
			b.WriteByte('\n')
		}
		if msg := strings.TrimSpace(in.Message); msg != "" {
			b.WriteString("\n补充说明：")
			b.WriteString(msg)
			b.WriteByte('\n')
		}
	} else {
		b.WriteString("【用户问题】\n")
		b.WriteString(in.Message)
		b.WriteByte('\n')
		if wantJSON {
			b.WriteString("\n请只返回 JSON 对象（key-value），不要其它文字。找不到填 null。\n")
		}
	}
	b.WriteString("\n【文件名】")
	b.WriteString(fileName)
	b.WriteByte('\n')
	if truncated {
		b.WriteString(fmt.Sprintf("【提示】文件过长，已截断至约 %d 字，请基于可见内容抽取。\n", maxRunes))
	}
	b.WriteString("\n【文件内容】\n")
	b.WriteString(fileText)
	return b.String()
}

func alignFields(data map[string]any, fields []string) map[string]any {
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if v, ok := data[f]; ok {
			out[f] = v
		} else {
			out[f] = nil
		}
	}
	return out
}

func parseJSONObject(s string) (map[string]any, error) {
	s = strings.TrimSpace(s)
	s = stripCodeFence(s)
	// 截取第一个 { 到最后一个 }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(s), &data); err != nil {
		return nil, err
	}
	return data, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func looksLikeResponseFormatError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "response_format") ||
		strings.Contains(msg, "json_object") ||
		strings.Contains(msg, "invalid_request")
}

func truncateForErr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

func truncateRunes(s string, max int) (string, bool) {
	r := []rune(s)
	if len(r) <= max {
		return s, false
	}
	return string(r[:max]), true
}
