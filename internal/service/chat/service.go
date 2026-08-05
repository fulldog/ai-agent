package chat

import (
	"context"
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
