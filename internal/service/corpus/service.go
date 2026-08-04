package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/database"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/embed"
	"github.com/webapp/go-app/ai-agent/pkg/chunker"
	"gorm.io/gorm"
)

type Service struct {
	db    *gorm.DB
	cfg   *config.Config
	embed *embed.Client
}

func New(db *gorm.DB, cfg *config.Config, embedClient *embed.Client) *Service {
	return &Service{db: db, cfg: cfg, embed: embedClient}
}

func (s *Service) Create(name, description string) (*model.Corpus, error) {
	c := &model.Corpus{
		Name:        name,
		Description: description,
		EmbedModel:  s.embed.Model(),
		EmbedDim:    s.embed.Dimensions(),
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Service) List() ([]model.Corpus, error) {
	var rows []model.Corpus
	err := s.db.Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) Get(id uuid.UUID) (*model.Corpus, error) {
	var c model.Corpus
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) Delete(id uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("corpus_id = ?", id).Delete(&model.Chunk{}).Error; err != nil {
			return err
		}
		if err := tx.Where("corpus_id = ?", id).Delete(&model.Document{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Corpus{}, "id = ?", id).Error
	})
}

type AddDocumentInput struct {
	CorpusID uuid.UUID
	Title    string
	Source   string
	Content  string
}

func (s *Service) AddDocument(ctx context.Context, in AddDocumentInput) (*model.Document, error) {
	sum := sha256.Sum256([]byte(in.Content))
	hash := hex.EncodeToString(sum[:])
	doc := &model.Document{
		CorpusID:    in.CorpusID,
		Title:       in.Title,
		Source:      in.Source,
		ContentHash: hash,
		Status:      "pending",
	}
	if err := s.db.Create(doc).Error; err != nil {
		return nil, err
	}
	if err := s.indexDocument(ctx, doc, in.Content); err != nil {
		_ = s.db.Model(doc).Updates(map[string]any{"status": "failed", "error_message": err.Error()}).Error
		return doc, err
	}
	_ = s.db.Model(doc).Updates(map[string]any{"status": "indexed", "error_message": ""}).Error
	doc.Status = "indexed"
	return doc, nil
}

func (s *Service) ListDocuments(corpusID uuid.UUID) ([]model.Document, error) {
	var rows []model.Document
	err := s.db.Where("corpus_id = ?", corpusID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) DeleteDocument(corpusID, docID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Chunk{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Document{}).Error
	})
}

func (s *Service) Reindex(ctx context.Context, corpusID uuid.UUID) error {
	docs, err := s.ListDocuments(corpusID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		var chunks []model.Chunk
		if err := s.db.Where("document_id = ?", d.ID).Order("chunk_index asc").Find(&chunks).Error; err != nil {
			return err
		}
		if len(chunks) == 0 {
			continue
		}
		var texts []string
		for _, ch := range chunks {
			texts = append(texts, ch.Content)
		}
		// rebuild from concatenated content for simplicity
		content := ""
		for i, t := range texts {
			if i > 0 {
				content += "\n"
			}
			content += t
		}
		if err := s.db.Where("document_id = ?", d.ID).Delete(&model.Chunk{}).Error; err != nil {
			return err
		}
		doc := d
		if err := s.indexDocument(ctx, &doc, content); err != nil {
			_ = s.db.Model(&doc).Updates(map[string]any{"status": "failed", "error_message": err.Error()}).Error
			return err
		}
		_ = s.db.Model(&doc).Updates(map[string]any{"status": "indexed", "error_message": ""}).Error
	}
	_ = database.EnsureVectorIndex(s.db, s.cfg.RAG.VectorIndex)
	return nil
}

func (s *Service) indexDocument(ctx context.Context, doc *model.Document, content string) error {
	parts := chunker.Split(content, s.cfg.RAG.ChunkSize, s.cfg.RAG.ChunkOverlap)
	if len(parts) == 0 {
		return fmt.Errorf("empty content")
	}
	vecs, err := s.embed.Embed(ctx, parts)
	if err != nil {
		return err
	}
	if len(vecs) != len(parts) {
		return fmt.Errorf("embedding count mismatch: %d vs %d", len(vecs), len(parts))
	}
	for i, part := range parts {
		ch := model.Chunk{
			CorpusID:   doc.CorpusID,
			DocumentID: doc.ID,
			ChunkIndex: i,
			Content:    part,
			Metadata:   "{}",
		}
		if err := s.db.Create(&ch).Error; err != nil {
			return err
		}
		lit := vectorLiteral(vecs[i])
		if err := s.db.Exec(`UPDATE chunks SET embedding = ?::vector WHERE id = ?`, lit, ch.ID).Error; err != nil {
			return err
		}
	}
	_ = database.EnsureVectorIndex(s.db, s.cfg.RAG.VectorIndex)
	return nil
}

func vectorLiteral(v []float32) string {
	b := make([]byte, 0, len(v)*8)
	b = append(b, '[')
	for i, f := range v {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, fmt.Sprintf("%g", f)...)
	}
	b = append(b, ']')
	return string(b)
}
