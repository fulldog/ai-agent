package llmog

import (
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

func Save(db *gorm.DB, row *model.LLMCallLog) {
	if db == nil || row == nil {
		return
	}
	_ = db.Create(row).Error
}

func PtrUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
