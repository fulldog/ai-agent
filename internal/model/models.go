package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Conversation 会话。
type Conversation struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey;comment:会话ID" json:"id"`
	UID          string         `gorm:"type:text;not null;default:'';index:idx_conversations_uid_created,priority:1;comment:所属用户UID(X-User-Id)" json:"uid"`
	Title        string         `gorm:"type:text;comment:会话标题" json:"title"`
	SystemPrompt string         `gorm:"type:text;comment:系统提示词" json:"system_prompt"`
	CorpusID     *uuid.UUID     `gorm:"type:uuid;index;comment:默认绑定的语料库ID(RAG)" json:"corpus_id,omitempty"`
	CreatedAt    time.Time      `gorm:"index:idx_conversations_uid_created,priority:2;comment:创建时间" json:"created_at"`
	UpdatedAt    time.Time      `gorm:"comment:更新时间" json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index;comment:软删除时间" json:"-"`
}

func (Conversation) TableName() string { return "conversations" }

func (c *Conversation) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// Message 会话消息。
type Message struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;comment:消息ID" json:"id"`
	ConversationID uuid.UUID `gorm:"type:uuid;index:idx_messages_conv_created,priority:1;comment:所属会话ID" json:"conversation_id"`
	Role           string    `gorm:"type:text;not null;comment:角色:system/user/assistant/tool" json:"role"`
	Content        string    `gorm:"type:text;comment:消息正文" json:"content"`
	ToolCallID     string    `gorm:"type:text;comment:tool消息关联的tool_call_id" json:"tool_call_id,omitempty"`
	// ToolCallsJSON 使用指针：空则写 NULL。空字符串对 jsonb 非法（SQLSTATE 22P02）。
	ToolCallsJSON   *string   `gorm:"type:jsonb;comment:assistant发起的tool_calls JSON" json:"tool_calls_json,omitempty"`
	TokenPrompt     *int      `gorm:"comment:本条消耗的prompt token" json:"token_prompt,omitempty"`
	TokenCompletion *int      `gorm:"comment:本条消耗的completion token" json:"token_completion,omitempty"`
	CreatedAt       time.Time `gorm:"index:idx_messages_conv_created,priority:2;comment:创建时间" json:"created_at"`
}

func (Message) TableName() string { return "messages" }

func (m *Message) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.ToolCallsJSON != nil && strings.TrimSpace(*m.ToolCallsJSON) == "" {
		m.ToolCallsJSON = nil
	}
	return nil
}

// Corpus 语料库。
type Corpus struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;comment:语料库ID" json:"id"`
	Name        string    `gorm:"type:text;uniqueIndex;not null;comment:语料库名称(唯一)" json:"name"`
	Description string    `gorm:"type:text;comment:语料库描述" json:"description"`
	EmbedModel  string    `gorm:"type:text;comment:Embedding模型名" json:"embed_model"`
	EmbedDim    int       `gorm:"comment:向量维度(如768)" json:"embed_dim"`
	CreatedAt   time.Time `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt   time.Time `gorm:"comment:更新时间" json:"updated_at"`
}

func (Corpus) TableName() string { return "corpora" }

func (c *Corpus) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// Document 语料文档。
type Document struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;comment:文档ID" json:"id"`
	CorpusID     uuid.UUID `gorm:"type:uuid;index:idx_documents_corpus,priority:1;comment:所属语料库ID" json:"corpus_id"`
	Title        string    `gorm:"type:text;comment:文档标题" json:"title"`
	Source       string    `gorm:"type:text;comment:来源(文件名/URL等)" json:"source"`
	ContentHash  string    `gorm:"type:text;comment:内容哈希(去重/变更检测)" json:"content_hash"`
	Status       string    `gorm:"type:text;index;comment:状态:pending/indexing/ready/failed" json:"status"`
	ErrorMessage string    `gorm:"type:text;comment:索引失败原因" json:"error_message,omitempty"`
	CreatedAt    time.Time `gorm:"index:idx_documents_corpus,priority:2;comment:创建时间" json:"created_at"`
	UpdatedAt    time.Time `gorm:"comment:更新时间" json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

func (d *Document) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}

// Chunk 文档分块；embedding 列由 raw SQL 维护(pgvector)。
type Chunk struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;comment:分块ID" json:"id"`
	CorpusID   uuid.UUID `gorm:"type:uuid;index;comment:所属语料库ID" json:"corpus_id"`
	DocumentID uuid.UUID `gorm:"type:uuid;index;comment:所属文档ID" json:"document_id"`
	ChunkIndex int       `gorm:"comment:文档内分块序号(从0起)" json:"chunk_index"`
	Content    string    `gorm:"type:text;comment:分块文本内容" json:"content"`
	Metadata   string    `gorm:"type:jsonb;default:'{}';comment:分块元数据JSON" json:"metadata"`
	CreatedAt  time.Time `gorm:"comment:创建时间" json:"created_at"`
}

func (Chunk) TableName() string { return "chunks" }

func (c *Chunk) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// AgentRun 一次 Agent 运行记录。
type AgentRun struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;comment:运行ID" json:"id"`
	ConversationID   *uuid.UUID `gorm:"type:uuid;index;comment:关联会话ID(可选)" json:"conversation_id,omitempty"`
	Input            string     `gorm:"type:text;comment:用户输入" json:"input"`
	Output           string     `gorm:"type:text;comment:最终输出" json:"output"`
	Model            string     `gorm:"type:text;comment:使用的模型名" json:"model"`
	Status           string     `gorm:"type:text;index;comment:状态:running/succeeded/failed" json:"status"`
	MaxSteps         int        `gorm:"comment:最大步数上限" json:"max_steps"`
	StepCount        int        `gorm:"comment:实际步数" json:"step_count"`
	ErrorMessage     string     `gorm:"type:text;comment:失败错误信息" json:"error_message,omitempty"`
	PromptTokens     int        `gorm:"comment:累计prompt token" json:"prompt_tokens"`
	CompletionTokens int        `gorm:"comment:累计completion token" json:"completion_tokens"`
	CreatedAt        time.Time  `gorm:"comment:创建时间" json:"created_at"`
	FinishedAt       *time.Time `gorm:"comment:结束时间" json:"finished_at,omitempty"`
}

func (AgentRun) TableName() string { return "agent_runs" }

func (a *AgentRun) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// AgentStep Agent 单步(LLM 或工具结果)。
type AgentStep struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;comment:步骤ID" json:"id"`
	RunID      uuid.UUID `gorm:"type:uuid;index:idx_agent_steps_run,priority:1;comment:所属运行ID" json:"run_id"`
	StepIndex  int       `gorm:"index:idx_agent_steps_run,priority:2;comment:步骤序号" json:"step_index"`
	Kind       string    `gorm:"type:text;comment:类型:llm/tool_result" json:"kind"`
	ToolName   string    `gorm:"type:text;comment:工具名(工具步骤时)" json:"tool_name,omitempty"`
	InputJSON  string    `gorm:"type:jsonb;comment:步骤输入JSON" json:"input_json,omitempty"`
	OutputText string    `gorm:"type:text;comment:步骤输出文本" json:"output_text,omitempty"`
	CreatedAt  time.Time `gorm:"comment:创建时间" json:"created_at"`
}

func (AgentStep) TableName() string { return "agent_steps" }

func (a *AgentStep) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

// RequestLog HTTP 请求日志。
type RequestLog struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;comment:日志ID" json:"id"`
	RequestID       string     `gorm:"type:text;uniqueIndex;comment:请求追踪ID" json:"request_id"`
	APIKeyID        string     `gorm:"type:text;comment:调用方API Key标识(脱敏)" json:"api_key_id"`
	Method          string     `gorm:"type:text;comment:HTTP方法" json:"method"`
	Path            string     `gorm:"type:text;comment:请求路径" json:"path"`
	PathTemplate    string     `gorm:"type:text;comment:路由模板" json:"path_template"`
	Status          int        `gorm:"comment:HTTP状态码" json:"status"`
	LatencyMs       int64      `gorm:"comment:耗时毫秒" json:"latency_ms"`
	RequestBody     string     `gorm:"type:text;comment:请求体摘要" json:"request_body,omitempty"`
	ResponsePreview string     `gorm:"type:text;comment:响应预览" json:"response_preview,omitempty"`
	Stream          bool       `gorm:"comment:是否SSE流式" json:"stream"`
	SSEEventCount   int        `gorm:"comment:SSE事件数" json:"sse_event_count"`
	ConversationID  *uuid.UUID `gorm:"type:uuid;index;comment:关联会话ID" json:"conversation_id,omitempty"`
	AgentRunID      *uuid.UUID `gorm:"type:uuid;index;comment:关联Agent运行ID" json:"agent_run_id,omitempty"`
	ErrorMessage    string     `gorm:"type:text;comment:错误信息" json:"error_message,omitempty"`
	CreatedAt       time.Time  `gorm:"index;comment:创建时间" json:"created_at"`
}

func (RequestLog) TableName() string { return "request_logs" }

func (r *RequestLog) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// LLMCallLog 上游大模型调用日志。
type LLMCallLog struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;comment:调用日志ID" json:"id"`
	RequestID        string     `gorm:"type:text;index;comment:关联HTTP请求ID" json:"request_id"`
	ConversationID   *uuid.UUID `gorm:"type:uuid;comment:关联会话ID" json:"conversation_id,omitempty"`
	AgentRunID       *uuid.UUID `gorm:"type:uuid;comment:关联Agent运行ID" json:"agent_run_id,omitempty"`
	Provider         string     `gorm:"type:text;comment:厂商:deepseek/qwen/kimi/doubao等" json:"provider"`
	Model            string     `gorm:"type:text;comment:模型名" json:"model"`
	Stream           bool       `gorm:"comment:是否流式" json:"stream"`
	Status           string     `gorm:"type:text;comment:状态:ok/error" json:"status"`
	PromptTokens     int        `gorm:"comment:prompt token数" json:"prompt_tokens"`
	CompletionTokens int        `gorm:"comment:completion token数" json:"completion_tokens"`
	LatencyMs        int64      `gorm:"comment:耗时毫秒" json:"latency_ms"`
	RequestSummary   string     `gorm:"type:text;comment:请求摘要" json:"request_summary,omitempty"`
	ErrorMessage     string     `gorm:"type:text;comment:错误信息" json:"error_message,omitempty"`
	CreatedAt        time.Time  `gorm:"index;comment:创建时间" json:"created_at"`
}

func (LLMCallLog) TableName() string { return "llm_call_logs" }

func (l *LLMCallLog) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

// FileExtraction 上传文件与抽取文本的关联（按内容哈希缓存；强制重读则软删旧行并新建）。
type FileExtraction struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;comment:抽取记录ID" json:"id"`
	ContentHash    string    `gorm:"type:text;index:idx_file_extractions_hash_active,priority:1;not null;comment:原始文件SHA256" json:"content_hash"`
	OriginalName   string    `gorm:"type:text;comment:上传文件名" json:"original_name"`
	Ext            string    `gorm:"type:text;comment:扩展名" json:"ext"`
	SizeBytes      int64     `gorm:"comment:原始文件字节数" json:"size_bytes"`
	SourcePath     string    `gorm:"type:text;comment:原始文件相对路径" json:"source_path"`
	TextPath       string    `gorm:"type:text;comment:抽取文本相对路径(可空)" json:"text_path"`
	TextChars      int       `gorm:"comment:抽取文本字符数" json:"text_chars"`
	ExtractBackend string    `gorm:"type:text;comment:抽取后端:local/kimi/qwen" json:"extract_backend"`
	RemoteFileID   string    `gorm:"type:text;comment:云端文件ID" json:"remote_file_id,omitempty"`
	Status         string    `gorm:"type:text;index;comment:状态:ready/failed" json:"status"`
	ErrorMessage   string    `gorm:"type:text;comment:失败原因" json:"error_message,omitempty"`
	IsDeleted      int16     `gorm:"type:smallint;not null;default:0;index:idx_file_extractions_hash_active,priority:2;comment:软删除:0有效1已删" json:"is_deleted"`
	CreatedAt      time.Time `gorm:"comment:创建时间" json:"created_at"`
	UpdatedAt      time.Time `gorm:"comment:更新时间" json:"updated_at"`
}

func (FileExtraction) TableName() string { return "file_extractions" }

func (f *FileExtraction) BeforeCreate(tx *gorm.DB) error {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return nil
}
