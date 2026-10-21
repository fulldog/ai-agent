package agent

import (
	"errors"
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
		Content:        executedSQLMessage(sqlText),
	}
	return db.Create(&msg).Error
}

func executedSQLMessage(sqlText string) string {
	return "执行 SQL：\n\n```sql\n" + sqlText + "\n```"
}

// LatestExecutedSQL 返回该会话最近一条实际执行的语句。没有时返回空字符串。
func LatestExecutedSQL(db *gorm.DB, conversationID uuid.UUID) (string, error) {
	if db == nil || conversationID == uuid.Nil {
		return "", nil
	}
	var row model.Message
	err := db.Where("conversation_id = ? AND role = ? AND content LIKE ?", conversationID, "tool", "执行 SQL：%").
		Order("created_at desc").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return parseExecutedSQL(row.Content), nil
}

func parseExecutedSQL(content string) string {
	const fence = "```sql\n"
	i := strings.Index(content, fence)
	if i < 0 {
		return ""
	}
	rest := content[i+len(fence):]
	if j := strings.Index(rest, "\n```"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}
