package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/metrics"
	"github.com/webapp/go-app/ai-agent/internal/service/embed"
	"gorm.io/gorm"
)

type Service struct {
	db          *gorm.DB
	embed       *embed.Client
	maxDistance float64
}

func New(db *gorm.DB, embedClient *embed.Client, maxDistance float64) *Service {
	return &Service{db: db, embed: embedClient, maxDistance: maxDistance}
}

// MaxDistance 余弦距离上限；<=0 表示不过滤。
func (s *Service) MaxDistance() float64 {
	if s == nil {
		return 0
	}
	return s.maxDistance
}

type Hit struct {
	ChunkID    uuid.UUID `json:"chunk_id"`
	DocumentID uuid.UUID `json:"document_id"`
	CorpusID   uuid.UUID `json:"corpus_id,omitempty"`
	Content    string    `json:"content"`
	Score      float64   `json:"score"`
	Metadata   string    `json:"metadata"`
}

func (s *Service) Search(ctx context.Context, corpusID uuid.UUID, query string, topK int) ([]Hit, error) {
	return s.SearchInCorpora(ctx, []uuid.UUID{corpusID}, query, topK)
}

// SearchInCorpora 在指定语料库中向量检索；corpusIDs 为空则检索全部已入库分块。
func (s *Service) SearchInCorpora(ctx context.Context, corpusIDs []uuid.UUID, query string, topK int) ([]Hit, error) {
	start := time.Now()
	status := "ok"
	defer func() {
		metrics.RAGSearch.WithLabelValues(status).Inc()
		metrics.RAGDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()
	if s.db == nil || s.embed == nil {
		status = "error"
		return nil, fmt.Errorf("rag not configured")
	}
	if topK <= 0 {
		topK = 5
	}
	vec, err := s.embed.EmbedOne(ctx, query)
	if err != nil {
		status = "error"
		return nil, err
	}
	vecLit := vectorLiteral(vec)
	type row struct {
		ID         uuid.UUID
		DocumentID uuid.UUID
		CorpusID   uuid.UUID
		Content    string
		Metadata   string
		Distance   float64
	}
	var rows []row
	args := []any{vecLit}
	where := "embedding IS NOT NULL"
	if len(corpusIDs) > 0 {
		ph := make([]string, len(corpusIDs))
		for i, id := range corpusIDs {
			ph[i] = "?"
			args = append(args, id)
		}
		where += " AND corpus_id IN (" + strings.Join(ph, ",") + ")"
	}
	args = append(args, vecLit, topK)
	q := fmt.Sprintf(`
SELECT id, document_id, corpus_id, content, COALESCE(metadata::text, '{}') AS metadata,
       (embedding <=> ?::vector) AS distance
FROM chunks
WHERE %s
ORDER BY embedding <=> ?::vector
LIMIT ?`, where)
	err = s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error
	if err != nil {
		status = "error"
		return nil, err
	}
	hits := make([]Hit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, Hit{
			ChunkID:    r.ID,
			DocumentID: r.DocumentID,
			CorpusID:   r.CorpusID,
			Content:    r.Content,
			Score:      r.Distance,
			Metadata:   r.Metadata,
		})
	}
	return FilterByMaxDistance(hits, s.maxDistance), nil
}

const HitsPromptHeader = "【知识摘录】只采用与问题直接相关的句子作答；材料不足就明确说不知道。不要整段照抄。"

// HitsPrompt 把召回片段拼进 system；空 hits 返回空串。
func HitsPrompt(hits []Hit) string {
	if len(hits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(HitsPromptHeader)
	b.WriteByte('\n')
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("[%d] %s\n", i+1, h.Content))
	}
	return strings.TrimSpace(b.String())
}

// FilterByMaxDistance 丢掉超过余弦距离上限的召回（Score 为 <=> 距离，越小越相似）。
// maxDistance <= 0 时不过滤。
func FilterByMaxDistance(hits []Hit, maxDistance float64) []Hit {
	if maxDistance <= 0 || len(hits) == 0 {
		return hits
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		if h.Score <= maxDistance {
			out = append(out, h)
		}
	}
	return out
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
