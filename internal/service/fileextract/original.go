package fileextract

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
)

// OpenOriginal 定位上传时落盘的原始文件。extractionID 优先；否则按原始文件名取最新一条。
func (s *Service) OpenOriginal(ctx context.Context, extractionID *uuid.UUID, originalName string) (string, string, error) {
	if s == nil || s.db == nil {
		return "", "", fmt.Errorf("fileextract 未配置")
	}
	var row model.FileExtraction
	if extractionID != nil && *extractionID != uuid.Nil {
		if err := s.db.WithContext(ctx).First(&row, "id = ?", *extractionID).Error; err != nil {
			return "", "", err
		}
	} else {
		name := strings.TrimSpace(originalName)
		base := filepath.Base(name)
		err := s.db.WithContext(ctx).
			Where("original_name = ? OR original_name = ?", name, base).
			Order("is_deleted asc, created_at desc").
			First(&row).Error
		if err != nil {
			return "", "", err
		}
	}
	abs, err := s.safeAbs(row.SourcePath)
	if err != nil {
		return "", "", err
	}
	name := strings.TrimSpace(row.OriginalName)
	if name == "" {
		name = filepath.Base(abs)
	}
	return abs, name, nil
}

func (s *Service) safeAbs(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" || s == nil || strings.TrimSpace(s.root) == "" {
		return "", fmt.Errorf("原始文件不存在")
	}
	cleaned := filepath.Clean(filepath.FromSlash(rel))
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || cleaned == ".." {
		return "", fmt.Errorf("invalid path")
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path")
	}
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("原始文件不存在")
	}
	return abs, nil
}
