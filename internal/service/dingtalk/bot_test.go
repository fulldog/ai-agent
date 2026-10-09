package dingtalk

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestUseAgentRequiresModeAndService(t *testing.T) {
	t.Parallel()
	if (&Bot{}).useAgent() {
		t.Fatal("empty bot")
	}
	b := &Bot{cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyAgent}}
	if b.useAgent() {
		t.Fatal("agent mode without service should fall back to chat")
	}
	b.agent = &agent.Service{}
	if !b.useAgent() {
		t.Fatal("want agent path")
	}
	b.cfg.ReplyMode = config.DingTalkReplyChat
	if b.useAgent() {
		t.Fatal("chat mode")
	}
}

func TestUseWebAgentRequiresModeAndService(t *testing.T) {
	t.Parallel()
	if (&Bot{}).useWebAgent() {
		t.Fatal("empty bot")
	}
	b := &Bot{cfg: config.DingTalkConfig{ReplyMode: config.DingTalkReplyWeb}}
	if b.useWebAgent() {
		t.Fatal("web mode without service should fall back")
	}
	b.agent = &agent.Service{}
	if !b.useWebAgent() {
		t.Fatal("want web agent path")
	}
	if b.useAgent() {
		t.Fatal("web mode must not select legacy agent path")
	}
	b.cfg.ReplyMode = config.DingTalkReplyAgent
	if b.useWebAgent() {
		t.Fatal("agent mode is not web")
	}
}

func TestPersistOutboundUpdatesSameAssistant(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Message{}, &model.RequestLog{}); err != nil {
		t.Fatal(err)
	}
	convID := uuid.New()
	raw := model.Message{ConversationID: convID, Role: "assistant", Content: "请补充供应商名称。"}
	if err := db.Create(&raw).Error; err != nil {
		t.Fatal(err)
	}
	b := &Bot{log: zap.NewNop(), db: db, previewMax: 200}
	decorated := raw.Content + "\n\n来源：SRM供应商系统\n\n〔LZ-SH-ZJ005〕"
	tr := &msgTrace{
		start:          time.Now(),
		requestID:      "msg-1",
		conversationID: &convID,
		assistantMsgID: &raw.ID,
		outcome:        "llm",
	}
	b.persistOutbound(tr, decorated)
	b.persistOutbound(tr, decorated)

	var rows []model.Message
	if err := db.Where("conversation_id = ? AND role = ?", convID, "assistant").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("assistant rows=%d, want 1", len(rows))
	}
	if rows[0].ID != raw.ID || rows[0].Content != decorated {
		t.Fatalf("got id=%s content=%q", rows[0].ID, rows[0].Content)
	}
}
