package dingtalk

import (
	"testing"

	"github.com/webapp/go-app/ai-agent/internal/config"
	"github.com/webapp/go-app/ai-agent/internal/service/agent"
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
