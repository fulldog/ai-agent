package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/webapp/go-app/ai-agent/internal/service/llm"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

// KnowledgeSearch 在 RAG 知识库中检索相关片段。
type KnowledgeSearch struct{}

func (KnowledgeSearch) Name() string { return "knowledge_search" }

func (KnowledgeSearch) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolSpecFunc{
			Name:        "knowledge_search",
			Description: "在知识库（RAG）中检索与问题相关的文本片段；未限定语料库时检索全部语料库",
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
	topK := env.TopK
	if topK <= 0 {
		topK = env.DefaultTopK
	}
	var (
		hits []rag.Hit
		err  error
	)
	if len(env.CorpusIDs) > 0 {
		hits, err = env.RAG.SearchInCorpora(ctx, env.CorpusIDs, p.Query, topK)
	} else if env.CorpusID != nil {
		hits, err = env.RAG.Search(ctx, *env.CorpusID, p.Query, topK)
	} else {
		hits, err = env.RAG.SearchInCorpora(ctx, nil, p.Query, topK)
	}
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		if d := env.RAG.MaxDistance(); d > 0 {
			return fmt.Sprintf("未检索到相关内容（余弦距离须 ≤ %.2f）", d), nil
		}
		return "未检索到相关内容", nil
	}
	var b strings.Builder
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("[%d] (distance=%.4f) %s\n", i+1, h.Score, h.Content))
	}
	return b.String(), nil
}
