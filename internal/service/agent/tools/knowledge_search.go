package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
)

// KnowledgeSearch 在 RAG 知识库中检索相关片段。
type KnowledgeSearch struct{}

func (KnowledgeSearch) Name() string { return "knowledge_search" }

func (KnowledgeSearch) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "knowledge_search",
			Description: "在知识库（RAG）中检索与问题相关的文本片段",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "检索查询词或问题"},
				},
				"required": []string{"query"},
			},
		},
	}
}

func (KnowledgeSearch) Exec(ctx context.Context, args json.RawMessage, env *Env) (string, error) {
	var p struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("参数无效: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return "", fmt.Errorf("query 不能为空")
	}
	if env == nil || env.RAG == nil {
		return "", fmt.Errorf("知识库服务未就绪")
	}
	if env.CorpusID == nil {
		return "", fmt.Errorf("knowledge_search 需要提供 corpus_id")
	}
	topK := env.TopK
	if topK <= 0 {
		topK = env.DefaultTopK
	}
	hits, err := env.RAG.Search(ctx, *env.CorpusID, p.Query, topK)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "未检索到相关内容", nil
	}
	var b strings.Builder
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("[%d] (distance=%.4f) %s\n", i+1, h.Score, h.Content))
	}
	return b.String(), nil
}
