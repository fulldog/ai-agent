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
	db    *gorm.DB
	embed *embed.Client
}

func New(db *gorm.DB, embedClient *embed.Client) *Service {
	return &Service{db: db, embed: embedClient}
}

type Hit struct {
	ChunkID    uuid.UUID `json:"chunk_id"`
	DocumentID uuid.UUID `json:"document_id"`
	Content    string    `json:"content"`
	Score      float64   `json:"score"`
	Metadata   string    `json:"metadata"`
}

func (s *Service) Search(ctx context.Context, corpusID uuid.UUID, query string, topK int) ([]Hit, error) {
	start := time.Now()
	status := "ok"
	defer func() {
		metrics.RAGSearch.WithLabelValues(status).Inc()
		metrics.RAGDuration.WithLabelValues(status).Observe(time.Since(start).Seconds())
	}()
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
		Content    string
		Metadata   string
		Distance   float64
	}
	var rows []row
	err = s.db.WithContext(ctx).Raw(`
SELECT id, document_id, content, COALESCE(metadata::text, '{}') AS metadata,
       (embedding <=> ?::vector) AS distance
FROM chunks
WHERE corpus_id = ? AND embedding IS NOT NULL
ORDER BY embedding <=> ?::vector
LIMIT ?`, vecLit, corpusID, vecLit, topK).Scan(&rows).Error
	if err != nil {
		status = "error"
		return nil, err
	}
	hits := make([]Hit, 0, len(rows))
	for _, r := range rows {
		hits = append(hits, Hit{
			ChunkID:    r.ID,
			DocumentID: r.DocumentID,
			Content:    r.Content,
			Score:      r.Distance,
			Metadata:   r.Metadata,
		})
	}
	return hits, nil
}

func vectorLiteral(v []float32) string {
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%g", f)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
