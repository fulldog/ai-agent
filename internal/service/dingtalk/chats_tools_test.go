package dingtalk

import (
	"errors"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/model"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
	"go.uber.org/zap"
)

func TestSetChatToolsRoundTrip(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{
		db:    db,
		log:   zap.NewNop(),
		agent: agent.New(db, nil, nil, nil, zap.NewNop(), nil, nil),
	}
	row := model.DingTalkChat{ConversationID: "cid-tools", Title: "工具群", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	item, err := b.SetChatTools(row.ID, []string{" knowledge_search ", "knowledge_search"})
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Tools) != 1 || item.Tools[0] != "knowledge_search" {
		t.Fatalf("stored %#v", item.Tools)
	}
	got := b.chatTools("cid-tools")
	if len(got) != 1 || got[0] != "knowledge_search" {
		t.Fatalf("loaded %#v", got)
	}
	cleared, err := b.SetChatTools(row.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Tools) != 0 || len(b.chatTools("cid-tools")) != 0 {
		t.Fatalf("cleared %#v", cleared.Tools)
	}
}

func TestSetChatToolsRejectsUnknown(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{
		db:    db,
		log:   zap.NewNop(),
		agent: agent.New(db, nil, nil, nil, zap.NewNop(), nil, nil),
	}
	row := model.DingTalkChat{ConversationID: "cid-bad-tool", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	_, err := b.SetChatTools(row.ID, []string{"not_a_tool"})
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("err: %v", err)
	}
}

func TestDecodeToolsInvalid(t *testing.T) {
	t.Parallel()
	if got := decodeTools("{"); len(got) != 0 {
		t.Fatalf("invalid json: %#v", got)
	}
}
