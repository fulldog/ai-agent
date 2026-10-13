package dingtalk

import (
	"context"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
	"go.uber.org/zap"
)

func (b *Bot) retrieve(ctx context.Context, query, conversationID string) (*uuid.UUID, []rag.Hit, []uuid.UUID) {
	if b.rag == nil || b.corpus == nil {
		return nil, nil, nil
	}
	bound := b.boundCorpora(conversationID)
	pool := bound
	if len(pool) == 0 {
		all, err := b.corpus.List()
		if err != nil {
			b.log.Warn("list corpora", zap.Error(err))
			return nil, nil, nil
		}
		if len(all) == 0 {
			return nil, nil, nil
		}
		pool = all
	}
	searchIn := corporaForRetrieve(pool, query)
	var pinned *uuid.UUID
	if matched := matchCorpora(query, searchIn); len(matched) > 0 {
		id := matched[0].ID
		pinned = &id
	}
	var scope []uuid.UUID
	if len(bound) > 0 {
		scope = corpusIDs(bound)
	}
	hits, err := b.rag.SearchInCorpora(ctx, corpusIDs(searchIn), query, b.ragTop)
	if err != nil {
		b.log.Warn("rag search", zap.Error(err))
		return pinned, []rag.Hit{}, scope
	}
	hits = filterRelevant(hits, b.maxDistance)
	if hits == nil {
		hits = []rag.Hit{}
	}
	if best := bestCorpusID(hits); best != nil {
		pinned = best
	}
	return pinned, hits, scope
}
