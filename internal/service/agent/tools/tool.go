package tools

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

// Tool 单个可调用工具。Description 与参数说明使用中文；Name 为英文 snake_case。
type Tool interface {
	Name() string
	Spec() llm.ToolSpec
	Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error)
}

// Env 执行期注入（语料、下游服务等）。
type Env struct {
	CorpusID    *uuid.UUID
	TopK        int
	DefaultTopK int
	RAG         *rag.Service
}
