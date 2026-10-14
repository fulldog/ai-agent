package dingtalk

import (
	"errors"
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/model"
	"go.uber.org/zap"
)

func TestSetChatHistoryLimit(t *testing.T) {
	db := openChatDB(t)
	b := &Bot{db: db, log: zap.NewNop()}
	missing := b.chatHistoryLimit("missing")
	if missing == nil || *missing != 0 {
		t.Fatalf("missing chat: %v", missing)
	}
	row := model.DingTalkChat{ConversationID: "cid-hist", Title: "历史群", ConversationType: "2"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	unset := b.chatHistoryLimit("cid-hist")
	if unset == nil || *unset != 0 {
		t.Fatalf("unset: %v", unset)
	}
	item, err := b.SetChatHistoryLimit(row.ID, 8)
	if err != nil {
		t.Fatal(err)
	}
	if item.HistoryLimit != 8 {
		t.Fatalf("stored %d", item.HistoryLimit)
	}
	got := b.chatHistoryLimit("cid-hist")
	if got == nil || *got != 8 {
		t.Fatalf("loaded %v", got)
	}
	if _, err := b.SetChatHistoryLimit(row.ID, 0); err != nil {
		t.Fatal(err)
	}
	cleared := b.chatHistoryLimit("cid-hist")
	if cleared == nil || *cleared != 0 {
		t.Fatalf("cleared %v", cleared)
	}
	if _, err := b.SetChatHistoryLimit(row.ID, -1); !errors.Is(err, ErrHistoryLimit) {
		t.Fatalf("negative: %v", err)
	}
	if _, err := b.SetChatHistoryLimit(row.ID, MaxHistoryLimit+1); !errors.Is(err, ErrHistoryLimit) {
		t.Fatalf("too big: %v", err)
	}
}
