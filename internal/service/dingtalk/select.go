package dingtalk

import (
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func matchCorpora(query string, corpora []model.Corpus) []model.Corpus {
	q := strings.TrimSpace(query)
	if q == "" || len(corpora) == 0 {
		return nil
	}
	seen := map[uuid.UUID]struct{}{}
	var out []model.Corpus
	for _, c := range corpora {
		name := strings.TrimSpace(c.Name)
		if name == "" || !strings.Contains(q, name) {
			continue
		}
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		out = append(out, c)
	}
	return out
}

func corpusIDs(rows []model.Corpus) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, c := range rows {
		ids = append(ids, c.ID)
	}
	return ids
}

func bestCorpusID(hits []rag.Hit) *uuid.UUID {
	for _, h := range hits {
		if h.CorpusID != uuid.Nil {
			id := h.CorpusID
			return &id
		}
	}
	return nil
}

func filterRelevant(hits []rag.Hit, maxDistance float64) []rag.Hit {
	return rag.FilterByMaxDistance(hits, maxDistance)
}

func isCorpusMiss(hits []rag.Hit, forceOnline bool) bool {
	return len(hits) == 0 && !forceOnline
}
