package dingtalk

import (
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/rag"
)

func corporaForRetrieve(pool []model.Corpus, query string) []model.Corpus {
	if matched := matchCorpora(query, pool); len(matched) > 0 {
		return matched
	}
	return pool
}

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

// unboundDirectChat 单聊且该会话没有绑定语料库。这种会话不检索知识库，直接请求大模型并打开联网。
func unboundDirectChat(conversationType string, bound int) bool {
	return !isGroup(conversationType) && bound == 0
}

// continueWithHistory 已有会话消息时，本轮语料未命中也继续作答（追问、补标签等）。
func continueWithHistory(hits []rag.Hit, forceOnline, hasHistory bool) bool {
	return hasHistory && isCorpusMiss(hits, forceOnline)
}

// stopForCorpusMiss 语料未命中、未要求联网、没有历史、也没有 dbconn 时直接结束。
// 挂了 dbconn 时继续交给模型，由模型决定是否查库。
func stopForCorpusMiss(hits []rag.Hit, forceOnline, hasHistory, hasDBConn bool) bool {
	return isCorpusMiss(hits, forceOnline) && !hasHistory && !hasDBConn
}
