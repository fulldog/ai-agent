package corpus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

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

func (s *Service) Update(id uuid.UUID, name, description string, setDescription bool) (*model.Corpus, error) {
	c, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	updates := map[string]any{"name": name}
	if setDescription {
		updates["description"] = description
	}
	if err := s.db.Model(c).Updates(updates).Error; err != nil {
		return nil, err
	}
	c.Name = name
	if setDescription {
		c.Description = description
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
		if err := tx.Where("corpus_id = ?", id).Delete(&model.DingTalkChatCorpus{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Corpus{}, "id = ?", id).Error
	})
}

type AddDocumentInput struct {
	CorpusID     uuid.UUID
	Title        string
	Source       string
	Content      string
	Kind         string
	ExtractionID *uuid.UUID
}

func (s *Service) AddDocument(ctx context.Context, in AddDocumentInput) (*model.Document, error) {
	if strings.TrimSpace(in.Content) == "" {
		return nil, fmt.Errorf("未能从文件中提取到正文")
	}
	sum := sha256.Sum256([]byte(in.Content))
	hash := hex.EncodeToString(sum[:])
	doc := &model.Document{
		CorpusID:     in.CorpusID,
		Title:        in.Title,
		Source:       in.Source,
		Kind:         ResolveKind(in.Kind, in.Source),
		ExtractionID: in.ExtractionID,
		Content:      in.Content,
		ContentHash:  hash,
		Status:       "pending",
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
	err := s.db.Omit("Content").Where("corpus_id = ?", corpusID).Order("created_at desc").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Kind = ResolveKind(rows[i].Kind, rows[i].Source)
	}
	return rows, nil
}

func (s *Service) GetDocument(corpusID, docID uuid.UUID) (*model.Document, error) {
	var doc model.Document
	if err := s.db.Where("id = ? AND corpus_id = ?", docID, corpusID).First(&doc).Error; err != nil {
		return nil, err
	}
	doc.Kind = ResolveKind(doc.Kind, doc.Source)
	return &doc, nil
}

// DocumentContent 返回原文。新文档读 content 列；旧文档按分块重叠拼回。
func (s *Service) DocumentContent(doc *model.Document) (string, error) {
	if doc == nil {
		return "", fmt.Errorf("document required")
	}
	if strings.TrimSpace(doc.Content) != "" {
		return doc.Content, nil
	}
	var chunks []model.Chunk
	if err := s.db.Where("document_id = ?", doc.ID).Order("chunk_index asc").Find(&chunks).Error; err != nil {
		return "", err
	}
	parts := make([]string, 0, len(chunks))
	for _, ch := range chunks {
		parts = append(parts, ch.Content)
	}
	overlap := 0
	if s.cfg != nil {
		overlap = s.cfg.RAG.ChunkOverlap
	}
	return joinChunks(parts, overlap), nil
}

func (s *Service) DeleteDocument(corpusID, docID uuid.UUID) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Chunk{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Document{}).Error
	})
}

// ReplaceDocument 用新正文替换一篇文档并只重建该文档的向量索引。
func (s *Service) ReplaceDocument(ctx context.Context, corpusID, docID uuid.UUID, in AddDocumentInput) (*model.Document, error) {
	var doc model.Document
	if err := s.db.Where("id = ? AND corpus_id = ?", docID, corpusID).First(&doc).Error; err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(in.Content))
	hash := hex.EncodeToString(sum[:])
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = doc.Title
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = doc.Source
	}
	kind := ResolveKind(in.Kind, source)
	if err := s.db.Model(&doc).Updates(map[string]any{
		"title":         title,
		"source":        source,
		"kind":          kind,
		"extraction_id": in.ExtractionID,
		"content":       in.Content,
		"content_hash":  hash,
		"status":        "pending",
		"error_message": "",
	}).Error; err != nil {
		return nil, err
	}
	doc.Title = title
	doc.Source = source
	doc.Kind = kind
	doc.ExtractionID = in.ExtractionID
	doc.Content = in.Content
	doc.ContentHash = hash
	doc.Status = "pending"
	doc.ErrorMessage = ""
	if err := s.db.Where("document_id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Chunk{}).Error; err != nil {
		return &doc, err
	}
	if err := s.indexDocument(ctx, &doc, in.Content); err != nil {
		_ = s.db.Where("document_id = ? AND corpus_id = ?", docID, corpusID).Delete(&model.Chunk{}).Error
		_ = s.db.Model(&doc).Updates(map[string]any{"status": "failed", "error_message": err.Error()}).Error
		doc.Status = "failed"
		doc.ErrorMessage = err.Error()
		return &doc, err
	}
	_ = s.db.Model(&doc).Updates(map[string]any{"status": "indexed", "error_message": ""}).Error
	doc.Status = "indexed"
	return &doc, nil
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
		var stored struct {
			Content string
		}
		if err := s.db.Model(&model.Document{}).Select("content").Where("id = ?", d.ID).Take(&stored).Error; err != nil {
			return err
		}
		content := strings.TrimSpace(stored.Content)
		if content == "" {
			parts := make([]string, len(chunks))
			for i, ch := range chunks {
				parts[i] = ch.Content
			}
			overlap := 0
			if s.cfg != nil {
				overlap = s.cfg.RAG.ChunkOverlap
			}
			content = joinChunks(parts, overlap)
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
