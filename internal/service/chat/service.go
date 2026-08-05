package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/llmog"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"gorm.io/gorm"
)

type Service struct {
	db   *gorm.DB
	cfg  *config.Config
	pool *llm.Pool
	rag  *rag.Service
}

func New(db *gorm.DB, cfg *config.Config, pool *llm.Pool, ragSvc *rag.Service) *Service {
	return &Service{db: db, cfg: cfg, pool: pool, rag: ragSvc}
}

type CreateConversationInput struct {
	Title        string
	SystemPrompt string
	CorpusID     *uuid.UUID
}

func (s *Service) CreateConversation(in CreateConversationInput) (*model.Conversation, error) {
	c := &model.Conversation{
		Title:        in.Title,
		SystemPrompt: in.SystemPrompt,
		CorpusID:     in.CorpusID,
	}
	if c.Title == "" {
		c.Title = "conversation"
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) ListConversations(limit, offset int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows []model.Conversation
	err := s.db.Order("created_at desc").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, err
}

func (s *Service) GetConversation(id uuid.UUID) (*model.Conversation, error) {
	var c model.Conversation
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) DeleteConversation(id uuid.UUID) error {
	return s.db.Delete(&model.Conversation{}, "id = ?", id).Error
}

func (s *Service) ListMessages(conversationID uuid.UUID, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 100
	}
	var rows []model.Message
	err := s.db.Where("conversation_id = ?", conversationID).Order("created_at asc").Limit(limit).Find(&rows).Error
	return rows, err
}

type CompleteInput struct {
	ConversationID uuid.UUID
	Message        string
	Provider       string
	Model          string
	Temperature    float64
	MaxTokens      int
	RAGEnabled     bool
	CorpusID       *uuid.UUID
	TopK           int
	RequestID      string
}

type CompleteResult struct {
	ConversationID   uuid.UUID
	MessageID        uuid.UUID
	Content          string
	PromptTokens     int
	CompletionTokens int
}

func (s *Service) Complete(ctx context.Context, in CompleteInput) (*CompleteResult, error) {
	conv, msgs, llmMsgs, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := client.Chat(ctx, llm.ChatRequest{
		Model:       modelName,
		Messages:    llmMsgs,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
	})
	status := "ok"
	errMsg := ""
	if err != nil {
		status = "error"
		errMsg = err.Error()
	}
	llmog.Save(s.db, &model.LLMCallLog{
		RequestID:        in.RequestID,
		ConversationID:   &conv.ID,
		Provider:         providerName,
		Model:            modelName,
		Stream:           false,
		Status:           status,
		PromptTokens:     valueOrZero(resp),
		CompletionTokens: completionOrZero(resp),
		LatencyMs:        time.Since(start).Milliseconds(),
		RequestSummary:   fmt.Sprintf("messages=%d", len(llmMsgs)),
		ErrorMessage:     errMsg,
	})
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
	_ = msgs
	return &CompleteResult{
		ConversationID:   conv.ID,
		MessageID:        asst.ID,
		Content:          resp.Content,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.CompletionTokens,
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
	userMsg := model.Message{ConversationID: conv.ID, Role: "user", Content: in.Message}
	if err := s.db.Create(&userMsg).Error; err != nil {
		return nil, err
	}
	start := time.Now()
	resp, err := client.ChatStream(ctx, llm.ChatRequest{
		Model:       modelName,
		Messages:    llmMsgs,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
	}, func(ev llm.StreamEvent) error {
		if ev.Content != "" && onDelta != nil {
			return onDelta(ev.Content)
		}
		return nil
	})
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
	llmog.Save(s.db, &model.LLMCallLog{
		RequestID:        in.RequestID,
		ConversationID:   &conv.ID,
		Provider:         providerName,
		Model:            modelName,
		Stream:           true,
		Status:           status,
		PromptTokens:     pt,
		CompletionTokens: ct,
		LatencyMs:        time.Since(start).Milliseconds(),
		RequestSummary:   fmt.Sprintf("messages=%d stream=true", len(llmMsgs)),
		ErrorMessage:     errMsg,
	})
	if err != nil {
		return nil, err
	}
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

func (s *Service) prepare(ctx context.Context, in CompleteInput) (*model.Conversation, []model.Message, []llm.Message, error) {
	conv, err := s.GetConversation(in.ConversationID)
	if err != nil {
		return nil, nil, nil, err
	}
	history, err := s.ListMessages(conv.ID, 100)
	if err != nil {
		return nil, nil, nil, err
	}
	var llmMsgs []llm.Message
	system := conv.SystemPrompt
	corpusID := in.CorpusID
	if corpusID == nil {
		corpusID = conv.CorpusID
	}
	if in.RAGEnabled && corpusID != nil && s.rag != nil {
		topK := in.TopK
		if topK <= 0 {
			topK = s.cfg.RAG.TopK
		}
		hits, rerr := s.rag.Search(ctx, *corpusID, in.Message, topK)
		if rerr == nil && len(hits) > 0 {
			var b strings.Builder
			b.WriteString("Use the following knowledge context when relevant:\n")
			for i, h := range hits {
				b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, h.Content))
			}
			if system != "" {
				system = system + "\n\n" + b.String()
			} else {
				system = b.String()
			}
		}
	}
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

func valueOrZero(r *llm.ChatResponse) int {
	if r == nil {
		return 0
	}
	return r.PromptTokens
}

func completionOrZero(r *llm.ChatResponse) int {
	if r == nil {
		return 0
	}
	return r.CompletionTokens
}

// 单次直传分析时，文件正文最大 rune 数（超出截断，避免撑爆上下文）。
const defaultAnalyzeMaxRunes = 80000

// AnalyzeInput 上传文件/正文，直接交给大模型分析（不入库语料库）。
type AnalyzeInput struct {
	ConversationID *uuid.UUID // 可选；有则落库消息
	Message        string     // 自定义问题；与 Fields 至少一个有值
	Fields         []string   // 要抽取的字段名，如 ["合同名称","合同编号"]
	ResponseJSON   bool       // 要求模型只返回 JSON 对象；有 Fields 时默认 true
	FileName       string
	FileText       string
	ContentHash    string
	ExtractionID   *uuid.UUID
	CacheHit       bool
	Provider       string
	Model          string
	Temperature    float64
	TempSet        bool // 是否显式传了 temperature（JSON 抽取默认 0）
	MaxTokens      int
	MaxFileRunes   int
	RequestID      string
	Stream         bool
}

type AnalyzeResult struct {
	ConversationID   *uuid.UUID
	MessageID        *uuid.UUID
	Content          string         // 模型原始文本
	Data             map[string]any // 解析后的 JSON 对象（response_json / fields 时）
	FileName         string
	FileChars        int
	Truncated        bool
	ContentHash      string
	ExtractionID     *uuid.UUID
	CacheHit         bool
	PromptTokens     int
	CompletionTokens int
}

func (s *Service) Analyze(ctx context.Context, in AnalyzeInput, onDelta func(string) error) (*AnalyzeResult, error) {
	wantJSON := in.ResponseJSON || len(in.Fields) > 0
	if strings.TrimSpace(in.Message) == "" && len(in.Fields) == 0 {
		return nil, fmt.Errorf("请提供 message（自定义问题）或 fields（要抽取的字段列表）")
	}
	if strings.TrimSpace(in.FileText) == "" {
		return nil, fmt.Errorf("文件内容为空，无法分析（请确认 PDF 有文字层，或已配置 OCR）")
	}

	maxRunes := in.MaxFileRunes
	if maxRunes <= 0 {
		maxRunes = defaultAnalyzeMaxRunes
	}
	fileText, truncated := truncateRunes(in.FileText, maxRunes)
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" {
		fileName = "upload"
	}

	userPrompt := buildAnalyzeUserPrompt(in, fileName, fileText, truncated, maxRunes, wantJSON)
	systemPrompt := "你是文档信息抽取助手。只依据用户提供的文件内容作答，不要编造。文件中找不到的字段填 null。"
	if wantJSON {
		systemPrompt = analyzeJSONSystemPrompt
	}

	client, providerName, modelName, err := s.pool.Resolve(in.Provider, in.Model)
	if err != nil {
		return nil, err
	}

	temp := in.Temperature
	if !in.TempSet && wantJSON {
		temp = 0
	}

	msgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	req := llm.ChatRequest{
		Model: modelName, Messages: msgs,
		Temperature: temp, MaxTokens: in.MaxTokens,
	}
	if wantJSON && !in.Stream {
		// 流式时部分厂商不支持 response_format，同步抽取时强制 JSON
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
		// 若厂商不支持 response_format，去掉后重试一次
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
	llmog.Save(s.db, &model.LLMCallLog{
		RequestID: in.RequestID, ConversationID: convID,
		Provider: providerName, Model: modelName, Stream: in.Stream, Status: status,
		PromptTokens: pt, CompletionTokens: ct, LatencyMs: time.Since(start).Milliseconds(),
		RequestSummary: fmt.Sprintf("analyze file=%s chars=%d truncated=%v json=%v fields=%d",
			fileName, len([]rune(fileText)), truncated, wantJSON, len(in.Fields)),
		ErrorMessage: errMsg,
	})
	if err != nil {
		return nil, err
	}

	out := &AnalyzeResult{
		Content: content, FileName: fileName, FileChars: len([]rune(fileText)),
		Truncated: truncated, PromptTokens: pt, CompletionTokens: ct,
		ContentHash: in.ContentHash, CacheHit: in.CacheHit, ExtractionID: in.ExtractionID,
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

	if in.ConversationID != nil {
		q := in.Message
		if q == "" && len(in.Fields) > 0 {
			q = "抽取字段: " + strings.Join(in.Fields, "、")
		}
		userContent := fmt.Sprintf("[文件分析] %s\n文件: %s (%d字)%s",
			q, fileName, out.FileChars, map[bool]string{true: "（已截断）", false: ""}[truncated])
		userMsg := model.Message{ConversationID: *in.ConversationID, Role: "user", Content: userContent}
		if err := s.db.Create(&userMsg).Error; err != nil {
			return nil, err
		}
		asstContent := content
		if out.Data != nil {
			if raw, mErr := json.Marshal(out.Data); mErr == nil {
				asstContent = string(raw)
			}
		}
		asst := model.Message{
			ConversationID: *in.ConversationID, Role: "assistant", Content: asstContent,
			TokenPrompt: &pt, TokenCompletion: &ct,
		}
		if err := s.db.Create(&asst).Error; err != nil {
			return nil, err
		}
		out.ConversationID = in.ConversationID
		id := asst.ID
		out.MessageID = &id
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
