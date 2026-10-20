package dingtalk

import (
	"errors"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"go.uber.org/zap"
)

func TestChatReplyModeOverridesGlobal(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{
		db:    db,
		log:   zap.NewNop(),
		cfg:   config.DingTalkConfig{ReplyMode: config.DingTalkReplyChat},
		agent: &agent.Service{},
	}
	one := model.DingTalkChat{ConversationID: "cid-one", ConversationType: "2"}
	two := model.DingTalkChat{ConversationID: "cid-two", ConversationType: "2"}
	if err := db.Create(&one).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&two).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := b.SetChatReplyMode(one.ID, "AGENT"); err != nil {
		t.Fatal(err)
	}
	if b.chatReplyMode("cid-one") != config.DingTalkReplyAgent || !b.useAgentFor("cid-one") {
		t.Fatalf("one: mode=%s agent=%v", b.chatReplyMode("cid-one"), b.useAgentFor("cid-one"))
	}
	if b.chatReplyMode("cid-two") != config.DingTalkReplyChat || b.useAgentFor("cid-two") || b.useWebAgentFor("cid-two") {
		t.Fatalf("two should stay global chat: %s", b.chatReplyMode("cid-two"))
	}

	b.cfg.ReplyMode = config.DingTalkReplyWeb
	if b.chatReplyMode("cid-two") != config.DingTalkReplyWeb || !b.useWebAgentFor("cid-two") {
		t.Fatalf("unset chat should follow global web: %s", b.chatReplyMode("cid-two"))
	}
	if b.chatReplyMode("cid-one") != config.DingTalkReplyAgent {
		t.Fatalf("override must ignore global, got %s", b.chatReplyMode("cid-one"))
	}

	if _, err := b.SetChatReplyMode(one.ID, "  "); err != nil {
		t.Fatal(err)
	}
	if b.chatReplyMode("cid-one") != config.DingTalkReplyWeb {
		t.Fatalf("cleared override should follow global, got %s", b.chatReplyMode("cid-one"))
	}

	if _, err := b.SetChatReplyMode(one.ID, "weird"); !errors.Is(err, ErrInvalidReplyMode) {
		t.Fatalf("err: %v", err)
	}
	var row model.DingTalkChat
	if err := db.First(&row, "id = ?", one.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.ReplyMode != "" {
		t.Fatalf("illegal value stored: %q", row.ReplyMode)
	}
	if err := db.Model(&model.DingTalkChat{}).Where("id = ?", one.ID).Update("reply_mode", "nope").Error; err != nil {
		t.Fatal(err)
	}
	if b.chatReplyMode("cid-one") != config.DingTalkReplyWeb {
		t.Fatalf("illegal stored value should fall back, got %s", b.chatReplyMode("cid-one"))
	}
}
