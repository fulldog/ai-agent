package fileextract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/pkg/extract"
	"gorm.io/gorm"
)

// Service 按文件内容哈希缓存抽取结果；强制重读时软删旧记录并新建（旧文件保留）。
type Service struct {
	db        *gorm.DB
	extractor *extract.Extractor
	root      string // attachments 根目录（相对或绝对）
}

func New(db *gorm.DB, extractor *extract.Extractor, attachmentsDir string) *Service {
	dir := strings.TrimSpace(attachmentsDir)
	if dir == "" {
		dir = "attachments"
	}
	return &Service{db: db, extractor: extractor, root: dir}
}

// Result 一次解析结果。
type Result struct {
	Text         string
	ExtractionID uuid.UUID
	ContentHash  string
	CacheHit     bool
	SourcePath   string
	TextPath     string
}

// Resolve 根据上传字节返回抽取文本：命中缓存则直接读盘；force 则软删旧行并重新抽取落盘。
func (s *Service) Resolve(ctx context.Context, filename string, data []byte, force bool) (*Result, error) {
	if s == nil || s.extractor == nil {
		return nil, fmt.Errorf("fileextract 未配置")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	hash := ContentHash(data)
	name := strings.TrimSpace(filename)
	if name == "" {
		name = "upload"
	}
	ext := strings.ToLower(filepath.Ext(name))

	if !force {
		if hit, err := s.loadActive(ctx, hash); err == nil && hit != nil {
			return hit, nil
		}
	} else {
		if err := s.softDeleteActive(ctx, hash); err != nil {
			return nil, err
		}
	}

	text, err := s.extractor.FromBytes(name, data)
	if err != nil {
		_ = s.insertFailed(ctx, hash, name, ext, int64(len(data)), err.Error())
		return nil, err
	}

	id := uuid.New()
	dayDir := time.Now().Format("2006/01/02")
	absDay := filepath.Join(s.root, filepath.FromSlash(dayDir))
	if err := os.MkdirAll(absDay, 0o755); err != nil {
		return nil, fmt.Errorf("创建附件目录失败: %w", err)
	}

	sourceName := id.String() + "_source" + ext
	textName := id.String() + ".txt"
	absSource := filepath.Join(absDay, sourceName)
	absText := filepath.Join(absDay, textName)
	relSource := filepath.ToSlash(filepath.Join(s.root, dayDir, sourceName))
	relText := filepath.ToSlash(filepath.Join(s.root, dayDir, textName))

	if err := os.WriteFile(absSource, data, 0o644); err != nil {
		return nil, fmt.Errorf("保存原始文件失败: %w", err)
	}
	if err := os.WriteFile(absText, []byte(text), 0o644); err != nil {
		return nil, fmt.Errorf("保存抽取文本失败: %w", err)
	}

	row := &model.FileExtraction{
		ID:           id,
		ContentHash:  hash,
		OriginalName: name,
		Ext:          ext,
		SizeBytes:    int64(len(data)),
		SourcePath:   relSource,
		TextPath:     relText,
		TextChars:    utf8.RuneCountInString(text),
		Status:       "ready",
		IsDeleted:    0,
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, fmt.Errorf("写入 file_extractions 失败: %w", err)
	}

	return &Result{
		Text:         text,
		ExtractionID: id,
		ContentHash:  hash,
		CacheHit:     false,
		SourcePath:   relSource,
		TextPath:     relText,
	}, nil
}

func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) loadActive(ctx context.Context, hash string) (*Result, error) {
	var row model.FileExtraction
	err := s.db.WithContext(ctx).
		Where("content_hash = ? AND is_deleted = 0 AND status = ?", hash, "ready").
		Order("created_at desc").
		First(&row).Error
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.FromSlash(row.TextPath))
	if err != nil {
		// 盘上丢失：软删该条，视为未命中
		_ = s.db.WithContext(ctx).Model(&model.FileExtraction{}).
			Where("id = ?", row.ID).
			Updates(map[string]any{"is_deleted": 1, "updated_at": time.Now()}).Error
		return nil, err
	}
	text := string(b)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("cached text empty")
	}
	return &Result{
		Text:         text,
		ExtractionID: row.ID,
		ContentHash:  hash,
		CacheHit:     true,
		SourcePath:   row.SourcePath,
		TextPath:     row.TextPath,
	}, nil
}

func (s *Service) softDeleteActive(ctx context.Context, hash string) error {
	now := time.Now()
	return s.db.WithContext(ctx).
		Model(&model.FileExtraction{}).
		Where("content_hash = ? AND is_deleted = 0", hash).
		Updates(map[string]any{
			"is_deleted": 1,
			"updated_at": now,
		}).Error
}

func (s *Service) insertFailed(ctx context.Context, hash, name, ext string, size int64, errMsg string) error {
	row := &model.FileExtraction{
		ContentHash:  hash,
		OriginalName: name,
		Ext:          ext,
		SizeBytes:    size,
		Status:       "failed",
		ErrorMessage: errMsg,
		IsDeleted:    0,
	}
	return s.db.WithContext(ctx).Create(row).Error
}
