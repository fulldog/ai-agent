package agent

import (
	"strings"

	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"gorm.io/gorm"
)

// SaveExecutedSQL 把 dbconn 实际执行的语句写入会话历史，供历史页查看。
// 角色用 tool，回放给模型时会跳过，避免孤立的 tool 消息。
func SaveExecutedSQL(db *gorm.DB, conversationID uuid.UUID, sqlText string) error {
	if db == nil || conversationID == uuid.Nil {
		return nil
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil
	}
	msg := model.Message{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        "执行 SQL：\n\n```sql\n" + sqlText + "\n```",
	}
	return db.Create(&msg).Error
}
