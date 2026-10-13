package agent

import (
	"context"

	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
)

func (s *Service) retrieveHits(ctx context.Context, in *RunInput) {
	if s == nil || in == nil || in.RAGHits != nil || s.rag == nil || in.SkipRAG {
		if in != nil && in.SkipRAG && in.RAGHits == nil {
			in.RAGHits = []rag.Hit{}
		}
		return
	}
	topK := in.TopK
	if topK <= 0 && s.cfg != nil {
		topK = s.cfg.RAG.TopK
	}
	var (
		hits []rag.Hit
		err  error
	)
	if len(in.CorpusIDs) > 0 {
		hits, err = s.rag.SearchInCorpora(ctx, in.CorpusIDs, in.Input, topK)
	} else if in.CorpusID != nil {
		hits, err = s.rag.Search(ctx, *in.CorpusID, in.Input, topK)
	} else {
		hits, err = s.rag.SearchInCorpora(ctx, nil, in.Input, topK)
	}
	if err != nil {
		if s.llmLog != nil {
			s.llmLog.Warn("agent corpus retrieve failed", zap.Error(err), zap.String("request_id", in.RequestID))
		}
		return
	}
	in.RAGHits = hits
}
