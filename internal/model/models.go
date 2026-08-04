package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Conversation struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Title        string         `gorm:"type:text" json:"title"`
	SystemPrompt string         `gorm:"type:text" json:"system_prompt"`
	CorpusID     *uuid.UUID     `gorm:"type:uuid;index" json:"corpus_id,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type Message struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID  uuid.UUID `gorm:"type:uuid;index:idx_messages_conv_created,priority:1" json:"conversation_id"`
	Role            string    `gorm:"type:text;not null" json:"role"`
	Content         string    `gorm:"type:text" json:"content"`
	ToolCallID      string    `gorm:"type:text" json:"tool_call_id,omitempty"`
	ToolCallsJSON   string    `gorm:"type:jsonb" json:"tool_calls_json,omitempty"`
	TokenPrompt     *int      `json:"token_prompt,omitempty"`
	TokenCompletion *int      `json:"token_completion,omitempty"`
	CreatedAt       time.Time `gorm:"index:idx_messages_conv_created,priority:2" json:"created_at"`
}

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

type Corpus struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string    `gorm:"type:text;uniqueIndex;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	EmbedModel  string    `gorm:"type:text" json:"embed_model"`
	EmbedDim    int       `json:"embed_dim"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (c *Corpus) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type Document struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CorpusID     uuid.UUID `gorm:"type:uuid;index:idx_documents_corpus,priority:1" json:"corpus_id"`
	Title        string    `gorm:"type:text" json:"title"`
	Source       string    `gorm:"type:text" json:"source"`
	ContentHash  string    `gorm:"type:text" json:"content_hash"`
	Status       string    `gorm:"type:text;index" json:"status"`
	ErrorMessage string    `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time `gorm:"index:idx_documents_corpus,priority:2" json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

// Chunk stores text pieces. Embedding is managed via raw SQL (pgvector).
type Chunk struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	CorpusID   uuid.UUID `gorm:"type:uuid;index" json:"corpus_id"`
	DocumentID uuid.UUID `gorm:"type:uuid;index" json:"document_id"`
	ChunkIndex int       `json:"chunk_index"`
	Content    string    `gorm:"type:text" json:"content"`
	Metadata   string    `gorm:"type:jsonb;default:'{}'" json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

func (c *Chunk) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

type AgentRun struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID   *uuid.UUID `gorm:"type:uuid;index" json:"conversation_id,omitempty"`
	Input            string     `gorm:"type:text" json:"input"`
	Output           string     `gorm:"type:text" json:"output"`
	Model            string     `gorm:"type:text" json:"model"`
	Status           string     `gorm:"type:text;index" json:"status"`
	MaxSteps         int        `json:"max_steps"`
	StepCount        int        `json:"step_count"`
	ErrorMessage     string     `gorm:"type:text" json:"error_message,omitempty"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

func (a *AgentRun) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type AgentStep struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	RunID      uuid.UUID `gorm:"type:uuid;index:idx_agent_steps_run,priority:1" json:"run_id"`
	StepIndex  int       `gorm:"index:idx_agent_steps_run,priority:2" json:"step_index"`
	Kind       string    `gorm:"type:text" json:"kind"`
	ToolName   string    `gorm:"type:text" json:"tool_name,omitempty"`
	InputJSON  string    `gorm:"type:jsonb" json:"input_json,omitempty"`
	OutputText string    `gorm:"type:text" json:"output_text,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func (a *AgentStep) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

type RequestLog struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RequestID       string     `gorm:"type:text;uniqueIndex" json:"request_id"`
	APIKeyID        string     `gorm:"type:text" json:"api_key_id"`
	Method          string     `gorm:"type:text" json:"method"`
	Path            string     `gorm:"type:text" json:"path"`
	PathTemplate    string     `gorm:"type:text" json:"path_template"`
	Status          int        `json:"status"`
	LatencyMs       int64      `json:"latency_ms"`
	RequestBody     string     `gorm:"type:text" json:"request_body,omitempty"`
	ResponsePreview string     `gorm:"type:text" json:"response_preview,omitempty"`
	Stream          bool       `json:"stream"`
	SSEEventCount   int        `json:"sse_event_count"`
	ConversationID  *uuid.UUID `gorm:"type:uuid;index" json:"conversation_id,omitempty"`
	AgentRunID      *uuid.UUID `gorm:"type:uuid;index" json:"agent_run_id,omitempty"`
	ErrorMessage    string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt       time.Time  `gorm:"index" json:"created_at"`
}

func (r *RequestLog) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

type LLMCallLog struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RequestID        string     `gorm:"type:text;index" json:"request_id"`
	ConversationID   *uuid.UUID `gorm:"type:uuid" json:"conversation_id,omitempty"`
	AgentRunID       *uuid.UUID `gorm:"type:uuid" json:"agent_run_id,omitempty"`
	Provider         string     `gorm:"type:text" json:"provider"`
	Model            string     `gorm:"type:text" json:"model"`
	Stream           bool       `json:"stream"`
	Status           string     `gorm:"type:text" json:"status"`
	PromptTokens     int        `json:"prompt_tokens"`
	CompletionTokens int        `json:"completion_tokens"`
	LatencyMs        int64      `json:"latency_ms"`
	RequestSummary   string     `gorm:"type:text" json:"request_summary,omitempty"`
	ErrorMessage     string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt        time.Time  `gorm:"index" json:"created_at"`
}

func (l *LLMCallLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}
