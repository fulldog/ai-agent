package dingtalk

import (
	"testing"

	"go.uber.org/zap"
)

func TestUpsertChatNoop(t *testing.T) {
	t.Parallel()
	b := &Bot{log: zap.NewNop()}
	b.upsertChat(nil)
	b.upsertChat(&botCallback{})
	b.upsertChat(&botCallback{ConversationID: "cid1", ConversationTitle: "采购群", ConversationType: "2"})
}

func TestTitleFromLegacyConversation(t *testing.T) {
	t.Parallel()
	title, typ := titleFromLegacyConversation("SRM AI 助手测试群 · 熊永坤")
	if title != "SRM AI 助手测试群" || typ != "2" {
		t.Fatalf("got %q %q", title, typ)
	}
	title, typ = titleFromLegacyConversation("钉钉单聊 · 张三")
	if title != "钉钉单聊" || typ != "1" {
		t.Fatalf("got %q %q", title, typ)
	}
	title, typ = titleFromLegacyConversation("钉钉群聊 · 李四")
	if title != "钉钉群聊" || typ != "2" {
		t.Fatalf("got %q %q", title, typ)
	}
}
