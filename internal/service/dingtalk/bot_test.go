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
